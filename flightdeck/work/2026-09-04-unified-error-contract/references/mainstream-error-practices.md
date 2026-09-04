# 主流前后端错误合同调研

> 调研范围：HTTP、OAuth 2、GraphQL、gRPC/Google API、Stripe、GitHub、OpenTelemetry，以及 Web 前端反馈与无障碍。本文只引用规范、标准组织和平台官方文档。调研日期：2026-09-04。

## 结论摘要

主流方案并不试图用一个字符串同时服务协议、程序分支、最终用户和运维人员。稳定的做法是把错误拆成四种信息：协议级粗分类（HTTP/gRPC status）、稳定且机器可读的原因标识、类型化的上下文/违规项，以及面向人类但不可供程序解析的文案。RFC 9457 明确说客户端不应解析 `detail`，Google AIP-193 则要求用 `ErrorInfo` 承载可依赖的机器信息；Stripe 和 GitHub 的实践也都是 HTTP 状态之外另给稳定 `code` 或结构化 `errors`。[RFC 9457 §3.1.4](https://www.rfc-editor.org/rfc/rfc9457.html#section-3.1.4) [Google AIP-193](https://google.aip.dev/193) [Stripe Errors](https://docs.stripe.com/api/errors) [GitHub REST troubleshooting](https://docs.github.com/en/rest/using-the-rest-api/troubleshooting-the-rest-api)

对 Yueli 最合适的方向是：继续以 RFC 9457 Problem Details 作为 HTTP 投影；在 Foundation 定义一份协议无关的 canonical failure；公共库拥有传输、校验、重试提示、追踪关联和安全降级，产品只声明业务原因、稳定参数和恢复动作。不要建立一个囊括所有产品错误的手写中央枚举，也不要让 UI 解析 `detail`、HTTP status 或机器码来猜文案、重试和呈现位置。

## 1. HTTP 与 RFC 9457

RFC 9457 定义 `application/problem+json`，核心成员是 `type`、`status`、`title`、`detail`、`instance`，并允许扩展成员；它取代了 RFC 7807。`type` 标识问题类型，`instance` 标识这一次具体发生，二者语义不可混用。[RFC 9457 §3](https://www.rfc-editor.org/rfc/rfc9457.html#section-3)

`status` 只是响应中真实 HTTP status 的 advisory 副本，生产方必须让两者一致；通用 HTTP 软件仍以状态行判断行为。因此网关 504、服务 503 或限流 429 不能被 JSON 解码器重写成“请求体无效”。[RFC 9457 §3.1.2](https://www.rfc-editor.org/rfc/rfc9457.html#section-3.1.2)

`title` 是问题类型的短摘要，除本地化外不应随实例改变；`detail` 是本次发生的人类可读说明，应帮助调用方纠正问题，客户端不应解析它。需要程序处理的数据应进入扩展成员。[RFC 9457 §3.1.3–3.1.4](https://www.rfc-editor.org/rfc/rfc9457.html#section-3.1.3)

新 problem type 至少应文档化稳定的 type URI、title 和推荐 HTTP status，type URI 最好可解析到解决说明；但纯通用 HTTP 情况通常直接用状态码即可，避免为每种 403/404 再造没有附加语义的类型。[RFC 9457 §4](https://www.rfc-editor.org/rfc/rfc9457.html#section-4)

RFC 9457 允许通过 `Accept-Language` 协商 `title`/`detail`，同时提醒 Problem Details 不是暴露实现调试信息的工具，定义问题类型时必须评估泄漏攻击面和实现细节的风险。[RFC 9457 §1](https://www.rfc-editor.org/rfc/rfc9457.html#section-1) [RFC 9457 §5](https://www.rfc-editor.org/rfc/rfc9457.html#section-5)

HTTP 方法是否可重试首先由语义决定：GET、PUT、DELETE 等幂等方法在通信失败后可重复；客户端不应自动重试非幂等方法，除非另有办法确认请求语义幂等或确认原请求未生效。`Retry-After` 可给出日期或等待秒数，503 时表示预计不可用时长。[RFC 9110 §9.2.2](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.2.2) [RFC 9110 §10.2.3](https://www.rfc-editor.org/rfc/rfc9110.html#section-10.2.3)

### 取舍

- Problem Details 与 HTTP 工具链自然兼容，扩展成本低；但自由扩展若没有产品 catalog 和 schema 门禁，会退化为各写各的键。
- `type` URI 标准而明确，但在封闭多仓体系内，用短稳定 `code` 做程序分支更方便。可以同时保留：`type` 负责公开类型身份，`code` 负责生成代码与 switch，二者由 catalog 一对一生成。
- 服务端本地化能服务无 UI 的 API 调用者，但会增加缓存、测试和日志比对复杂度；客户端本地化能保证产品语气一致。因此 Foundation 应保证稳定 code/params，并允许但不依赖服务端 `title/detail` 本地化。

## 2. OAuth 2 错误响应是专用协议合同

OAuth 2 的 token endpoint 错误不是 RFC 9457，而是固定的 `error`、可选 `error_description`、可选 `error_uri`；通常返回 400，`invalid_client` 在使用 HTTP Authentication 时返回 401 并带 `WWW-Authenticate`。`error` 是受限 ASCII 代码，`error_description` 面向开发者且不可作为机器分支依据。[RFC 6749 §5.2](https://www.rfc-editor.org/rfc/rfc6749.html#section-5.2)

authorization endpoint 的错误通过 redirect 参数返回，并要求带回请求中的 `state`；若 redirect URI 缺失、无效或不匹配，授权服务器不得自动重定向到该 URI，以免把敏感信息发送给攻击者。[RFC 6749 §4.1.2.1](https://www.rfc-editor.org/rfc/rfc6749.html#section-4.1.2.1)

### 对 Yueli 的约束

Identity 的 OAuth/OIDC 边界必须保持协议规定的 wire format，不能为了“全站统一”强行改成 Problem Details。统一应发生在内部 canonical failure 和前端 resolver 层；协议 adapter 负责投影为 OAuth error，并保留 `state`、认证头和 redirect 安全语义。

## 3. GraphQL：错误可与部分数据同时存在

GraphQL response 顶层只允许 `data`、`errors`、`extensions`。请求在执行前因解析、校验或变量错误失败时不应有 `data`；字段执行失败时可以继续执行并同时返回部分 `data` 与 `errors`。[GraphQL Specification §7.1](https://spec.graphql.org/October2021/#sec-Response-Format)

每个错误必须有开发者可读的 `message`；能定位到查询文档时应有 `locations`，能定位到结果字段时必须有 `path`。自定义机器信息应放入错误的 `extensions` map，不应在错误对象顶层随意加键。[GraphQL Specification §7.1.2](https://spec.graphql.org/October2021/#sec-Errors)

GraphQL over HTTP 草案区分 GraphQL request/execution result 与 HTTP 传输故障，并要求支持 `application/graphql-response+json`；它仍处于 draft，采用时应把版本状态写进合同。[GraphQL over HTTP §6.1](https://graphql.github.io/graphql-over-http/draft/#sec-Body)

### 取舍

GraphQL 的 `path` 和部分成功适合聚合查询，却让“一次请求成功/失败”的二元 UI 模型失效。Yueli 若引入 GraphQL，必须让 resolver 按字段路径呈现失败，并把稳定 `code/category/retry` 放在 `extensions`；不能把整个 HTTP 200 当成业务成功。REST 批量接口则不应随意模仿部分成功：Google AIP-193 建议通常避免 partial errors，确有必要的批处理用长任务并让每项错误仍保持结构化 Status。[Google AIP-193, Partial errors](https://google.aip.dev/193#partial-errors)

## 4. gRPC、`google.rpc.Status` 与 AIP-193

基础 gRPC 模型提供 canonical status code 和可选 message；Google rich error model 在 `google.rpc.Status.details` 中附加 protobuf detail，并由 trailing metadata 传输。官方 gRPC 文档提醒 rich model 的语言支持并不完全一致，grpc-web/Node 等消费者采用前必须验证实际 SDK 能力。[gRPC Error handling](https://grpc.io/docs/guides/error/)

`google.rpc.Status` 由 `code`、开发者可读英文 `message` 和类型化 `details[]` 组成。标准 detail 包括：`ErrorInfo`（domain/reason/metadata）、`BadRequest`（字段违规）、`RetryInfo`（最小重试等待）、`QuotaFailure`、`PreconditionFailure`、`ResourceInfo`、`RequestInfo` 和 `LocalizedMessage`。[google.rpc reference](https://cloud.google.com/storage/docs/reference/rpc/google.rpc)

AIP-193 要求使用 canonical `google.rpc.Code`，并要求 `ErrorInfo` 进一步区分有限的粗粒度 status；reason/domain 必须稳定，metadata 只能向后兼容地扩展。只要一直提供机器可读 `ErrorInfo`，人类 message 就可以演进，否则客户端可能被迫解析 message，使文案事实上成为兼容合同。[Google AIP-193](https://google.aip.dev/193)

Google 将 `Status.message` 定位为开发者可读英文；最终用户文案应放在 `LocalizedMessage` 或由客户端本地化。`RetryInfo.retry_delay` 表示再次尝试前至少等待多久，连续失败仍应采用指数退避。[google.rpc Status and RetryInfo](https://cloud.google.com/storage/docs/reference/rpc/google.rpc)

### 取舍

- Protobuf `Any` 支持强类型、跨语言生成和向后扩展，但 Web/TypeScript 端解包复杂，且 rich details 并非所有 gRPC 实现都支持。
- Yueli 不需要把 HTTP Problem 直接做成 `google.rpc.Status`；应让两者都是 canonical failure 的 adapter。字段违规、retry hint、resource、help link 等概念可借鉴标准 detail 的语义和命名。
- canonical failure 的 `category` 应保持小而稳定，类似 gRPC code；产品 `code` 则类似 `ErrorInfo.reason`，提供细粒度机器分支。两者不能互相替代。

## 5. Stripe 与 GitHub 的生产 API 实践

Stripe 使用常规 HTTP status 做粗分类，并在错误对象中提供 `type`、可选 `code`、`param`、支付专用 `decline_code`，以及指向该 code 官方说明的 `doc_url`；错误 code 有公开目录，Dashboard/Workbench 保留每次成功或失败的 API 请求供排查。[Stripe Errors API](https://docs.stripe.com/api/errors) [Stripe Error codes](https://docs.stripe.com/error-codes)

Stripe 的 idempotency key 让创建/更新请求可安全重试：首次请求开始执行后，服务端保存 status 和 body，后续同 key 返回相同结果，包括首次 500；参数校验失败或与并发请求冲突而尚未开始执行时不保存结果。这个细节说明“retryable”不能仅由 500 推断，必须与幂等键生命周期和是否已开始执行共同判断。[Stripe Idempotent requests](https://docs.stripe.com/api/idempotent_requests)

GitHub 的 422 validation response 包含 `errors` 数组和稳定 code（如 `missing_field`、`invalid`、`already_exists`）；限流时结合 403/429、`Retry-After`、`X-RateLimit-Remaining` 和 `X-RateLimit-Reset` 指导等待，持续失败时使用指数递增等待并最终停止。响应还暴露 `X-GitHub-Request-Id` 用于请求关联。[GitHub REST troubleshooting](https://docs.github.com/en/rest/using-the-rest-api/troubleshooting-the-rest-api) [GitHub REST getting started](https://docs.github.com/en/rest/using-the-rest-api/getting-started-with-the-rest-api)

GitHub 对私有资源可能返回 404 而非 403，以避免确认资源存在。这表明 status/code 映射也包含安全策略，客户端不得把 404 一概翻译成“资源一定不存在”。[GitHub REST troubleshooting](https://docs.github.com/en/rest/using-the-rest-api/troubleshooting-the-rest-api#404-not-found-for-an-existing-resource)

### 可借鉴点

- 像 Stripe 一样让每个公开 code 有文档链接和测试入口；像 GitHub 一样让限流/重试信息也出现在标准 HTTP header 中。
- `param` 只适合单字段错误；复杂表单和批量输入应使用 violations 数组，包含稳定字段路径、原因 code 和安全参数。
- 对外隐藏资源存在性属于服务端授权合同，不应由前端根据 403/404 自行猜测。

## 6. 可观测性与关联 ID

OpenTelemetry 的 context propagation 让 trace context 跨服务传播；Logs data model 的 `TraceId`/`SpanId` 可以把同一次执行涉及的跨组件日志与 trace 直接关联。[OpenTelemetry Context propagation](https://opentelemetry.io/docs/concepts/context-propagation/) [OpenTelemetry Logs specification](https://opentelemetry.io/docs/specs/otel/logs/#log-correlation)

错误响应中的公开 `traceId` 应是可复制的排障关联值，但不等于面向产品流程的 `operationId`：trace 可能因异步队列、重试或长任务分成多个 trace；operation/run/batch ID 则用于把多个请求和持久任务串成一次用户动作。这是基于 OpenTelemetry trace 语义与异步业务生命周期差异得出的设计推论。

### 对 Yueli 的约束

- 入站合法 trace context 应继续传播；日志必须记录 trace/span，公开错误至少返回一个可供支持人员检索的关联 ID。
- canonical failure 可同时持有 `traceId`、`operationId`、`durableId`，但三者语义、生成者和生命周期必须文档化，禁止把用户输入原样当可信 trace ID。
- UI 的“复制错误信息”应默认只包含稳定 code、关联 ID、发生时间和安全上下文，不包含 token、请求体、文件绝对路径、SQL、堆栈或上游原文。

## 7. 校验违规与前端反馈

Google 的 `BadRequest.FieldViolation` 表达字段路径与违规说明，GitHub 的 validation `errors` 则额外给机器 code；两者共同说明校验失败应支持多个、可定位、结构化的违规项，而不是只有一个 `invalid_input`。[google.rpc BadRequest](https://cloud.google.com/storage/docs/reference/rpc/google.rpc#badrequest) [GitHub REST troubleshooting](https://docs.github.com/en/rest/using-the-rest-api/troubleshooting-the-rest-api#validation-failed)

W3C 建议把字段错误与控件通过 `aria-describedby` 关联，并用 `aria-invalid` 标识失败字段；动态注入的重要错误可以放入预先存在的 `role=alert`/live region，使辅助技术无需移动焦点即可获知更新。[WAI Form notifications](https://www.w3.org/WAI/tutorials/forms/notifications/) [WCAG Technique ARIA19](https://www.w3.org/WAI/WCAG22/Techniques/aria/ARIA19) [WCAG Technique ARIA21](https://www.w3.org/WAI/WCAG21/Techniques/aria/ARIA21.html)

W3C 同时指出 `alert`/`aria-live=assertive` 只应给重要且时间敏感的信息，滥用本身会造成无障碍失败。[WCAG 4.1.3 understanding](https://www.w3.org/WAI/WCAG21/Understanding/status-messages)

### UI resolver 建议

1. `violations[].path` 能映射控件时，显示 inline error、设置 `aria-invalid` 并关联说明；提交后聚焦第一个无效控件或可导航的错误摘要。
2. 影响整个区域但不紧急的错误用持久 Alert；短暂操作反馈用 Toast/`role=status`；阻断且时间敏感的失败才使用 `role=alert`。
3. 机器 code 永远不直接作为主文案。缺少翻译时使用按 category/action 选择的安全兜底，开发环境或“详情”区域再显示 code。
4. `retryable=true` 也不等于立即自动重试：UI 还需检查请求幂等性、用户意图、`retryAfter` 和最大次数；产生副作用的重试必须有 idempotency key 或明确确认。

## 8. 本地化与安全边界

稳定的机器字段与可变的人类文案必须分离。RFC 9457 允许本地化 `title/detail` 且禁止客户端解析 `detail`；Google 要求开发者 message 与 `LocalizedMessage`/客户端本地化分离；OAuth 2 的 `error_description` 也是辅助开发者理解，而 `error` 才是协议代码。[RFC 9457 §3.1](https://www.rfc-editor.org/rfc/rfc9457.html#section-3.1) [Google AIP-193](https://google.aip.dev/193) [RFC 6749 §5.2](https://www.rfc-editor.org/rfc/rfc6749.html#section-5.2)

公开错误不能泄漏实现细节。RFC 9457 将 Problem Details 定位为 HTTP 接口信息而非底层调试工具；Google 官方错误指南同样要求避免在 message/details 泄漏敏感用户数据和服务端策略，并明确 `DebugInfo` 只应用于服务端日志、不得发送给客户端。[RFC 9457 §4–5](https://www.rfc-editor.org/rfc/rfc9457.html#section-4) [Google API errors guidance](https://cloud.google.com/distributed-cloud/hosted/docs/latest/gdcag/apis/errors#generate_errors)

因此 params/metadata 不能接收任意 `err.Error()`。每个 code 必须声明允许的参数名、类型、敏感级别和最大长度；未知 cause 只映射到安全内部错误并记录服务端 cause。路径应使用领域字段路径，不能回传本地绝对路径；上游 provider body、SQL、stack、token、Cookie 和对象存储内部 key 均不得进入客户端合同。

## 9. 治理、schema 与代码生成

上述标准共同依赖稳定机器标识和类型化扩展：RFC 9457 要求问题类型有稳定文档，AIP-193 要求 reason/domain 稳定且 metadata 只扩展，Stripe 维护公开 code 目录，GraphQL 把自定义字段约束在 `extensions`。[RFC 9457 §4](https://www.rfc-editor.org/rfc/rfc9457.html#section-4) [Google AIP-193](https://google.aip.dev/193) [Stripe Error codes](https://docs.stripe.com/error-codes) [GraphQL Specification §7.1.2](https://spec.graphql.org/October2021/#sec-Errors)

对多仓环境，手写 Go descriptor、TS union、JSON catalog、OpenAPI schema 和 i18n key 会必然漂移。推荐把每个产品的一份声明式 catalog 作为单一来源，由生成器产生：

- Go typed constructor/descriptor 与允许参数校验；
- TypeScript code/category union、params 类型和 resolver metadata；
- RFC 9457 JSON Schema/OpenAPI component；
- 中英文 i18n key inventory 和文档页；
- OAuth、HTTP、gRPC、任务记录等 adapter 的映射表；
- 合同测试 fixture，以及 breaking-change diff。

生成不等于把所有业务错误收归 Foundation。Foundation catalog 只容纳真正跨产品、语义完全一致的公共失败；产品 catalog 拥有本域错误。生成器应拒绝重复 code、未知 category、非法 status、未声明 params、缺少必需 locale、缺失恢复动作、将 5xx 标为字段违规等矛盾。

## 10. 推荐的 Yueli canonical failure

下面不是要直接暴露给所有协议的 JSON，而是内部语义模型；各 adapter 只投影协议允许的部分。

```text
Failure {
  code            // 稳定、细粒度、产品拥有，例如 docs.import.compression_unsupported
  category        // 小而稳定：validation/domain/policy/dependency/infrastructure/protocol
  params          // 按 code 声明并验证的安全机器参数
  violations[]    // path + code + params，支持多字段/批量项
  retry           // never | safe | after；可含 retryAfter，仍受幂等合同约束
  traceId         // 单次分布式执行关联
  operationId     // 一次用户动作关联，可跨请求/重试
  durableId       // run/job/batch 等持久实体 ID（若存在）
  help            // 可选稳定帮助链接/恢复动作 ID
  cause           // 仅进服务端日志，永不投影给客户端
}
```

HTTP adapter 投影为 RFC 9457：真实 status 与 `status` 一致，`type` 指向稳定文档，`instance` 表示本次发生，扩展包含 `code/category/params/violations/retry/traceId/operationId`。OAuth adapter 保持 RFC 6749；GraphQL adapter 把 code 等放入 `errors[].extensions`；gRPC adapter 映射 canonical code 并优先使用标准 `google.rpc` details；后台任务持久化同一语义模型的安全快照。

## 11. Foundation 必须落实的设计约束

1. **协议状态优先且不可伪造。** HTTP/gRPC/OAuth adapter 必须符合各自规范；HTML 502/503/504、网络中断和超时归为 transport/upstream failure，不得被解析失败覆盖成 `invalid_body`。
2. **一次语义映射。** typed domain cause 在最了解业务的 seam 映射一次；外层 adapter 只投影，不再猜业务 code。
3. **机器合同与文案分离。** 客户端只能分支于 code/category/typed params/retry，不得解析 title/detail/message；UI 文案由 locale catalog 解析，服务端文本只作兜底或开发者信息。
4. **重试是显式能力，不是 status 猜测。** 同时考虑 retry hint、HTTP 方法/业务幂等性、idempotency key、是否已开始执行、`Retry-After` 与有上限的指数退避。
5. **校验必须可定位且可批量。** violations 使用稳定领域路径和子 code；支持字段、列表索引和批量项，但不暴露内部结构或绝对路径。
6. **关联 ID 分层。** trace、operation、durable ID 分别定义生命周期；服务端日志可由返回 ID 检索，UI 提供安全复制入口。
7. **安全默认失败。** 未知 cause、无结构上游响应和 schema 不匹配都映射为安全公共错误；原始 cause 只写受控日志。授权边界允许为了防枚举隐藏资源存在性。
8. **产品拥有业务语义。** Foundation 拥有模型、runtime、adapter、通用 code、生成器和门禁；产品拥有业务 code、params、映射 seam、文案和恢复动作。
9. **声明生成、多仓验收。** CI 校验 catalog 唯一性、schema、locale 完整性、status/category 一致性、向后兼容和 UI 不直出机器码；生成 Go/TS/OpenAPI/i18n/docs，禁止重复手写映射表。
10. **部分成功必须显式建模。** 普通 REST 操作默认原子失败；批量/长任务确需部分成功时，每项都有结构化 failure 和稳定 durable ID；GraphQL 消费者必须处理 `data + errors`。

## 12. 建议的第一阶段落地范围

- 先冻结 canonical fields、category、retry 语义、violation path 语法和 ID 生命周期，不急于迁移全部 code。
- 在 Foundation 建 catalog schema、Go/TS generator、HTTP Problem adapter、前端 resolver 与合同测试。
- 选 Docs 导入（批量/文件/后台任务）、Identity（OAuth 专用协议）、Asset（上传/上游依赖）作为三个代表消费者。
- 第一阶段门禁只禁止新增漂移：新公开错误必须入 catalog、必须有中英文、不得回传 cause、UI 不得直接展示机器码；存量逐 Work 迁移。
- 用真实网关 HTML 504、空响应、错误 content-type、字段多违规、429 + Retry-After、幂等重试、OAuth redirect error、异步任务失败和缺失翻译作为跨仓验收 fixture。

这套边界吸收了 RFC 9457 的 HTTP 互操作性、Google rich details 的类型化语义、Stripe 的幂等重试、GitHub 的限流与 validation 经验、GraphQL 的路径/部分结果模型，以及 OpenTelemetry 的追踪关联，同时避免把任一协议特有 envelope 误当成所有场景的唯一领域模型。
