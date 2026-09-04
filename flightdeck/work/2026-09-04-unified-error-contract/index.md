# 统一 HTTP 结果合同

## Goal

交付一个由 Foundation 拥有的深 HTTP Result Module：统一成功响应模型、跨语言失败合同、产品错误目录生成、传输降级、前端反馈与治理门禁，同时保留产品对 DTO、业务错误和恢复动作的所有权。

## Status

Open

## Current

Foundation 已交付 Project v1 通用生成器、JSON Schema 和独立 Operation Errors v1。`httpcontract -project` 可运行产品 OpenAPI producer，从真实 OpenAPI 投影完整 operation，校验失效错误路由和 catalog 使用覆盖，并生成 Go/TypeScript/i18n/legacy；`-check` 使用临时 OpenAPI防止覆盖后自证。产品只拥有 project config 与业务 operation-errors 文件。

主流一手资料调研已完成。RFC 9457、OAuth 2、GraphQL、gRPC/Google AIP-193、Stripe、GitHub、OpenTelemetry 与 W3C 的共同方向是：协议状态、稳定机器原因、类型化上下文、最终用户文案和内部 cause 必须分离；重试需要同时考虑幂等性、执行状态和服务端 hint；字段违规需要可定位；trace、operation 与 durable identity 必须各自定义生命周期。研究已形成 canonical failure 候选模型、10 条设计约束和首阶段范围。

Interface 已完成三案比较并定稿：不引入 `Result<T>` 或 `{code,data,message}` 成功 Envelope；普通成功返回原始 DTO，集合统一 `items`，201/202/204 显式表达；失败直接深化现有 HTTP Problem v1。构建时以产品声明式 catalog 生成多语言产物和门禁，运行时由现有 Adapter 隐藏 status、trace、校验与安全降级。

第一段实现已落地：`contracts/http-result` 定义错误目录与 operation 成功形状 Schema；Go `httpcontract` 提供严格解析、namespace/status/参数/violations/成功形状校验、跨 manifest 引用检查、CLI 和确定性 Go catalog 生成。支持 raw resource、`{items}`、页码/游标集合、201 创建、202 operation、204 empty、binary 与 redirect，并编码互斥规则。Docs 试点 manifest 已通过 validator，其手写 code/status map 已由生成文件替换；过程中发现并补回旧公开 catalog 遗漏的 `docs.administrator_grant_protected`。

构建时 Interface 已补齐 compatibility diff、TypeScript discriminated union、i18n inventory 和 committed-output `-check`；Foundation Go 全量与 HTTP Runtime 19 tests 通过。Docs 试点已实际兑现集合创建 raw DTO + 201、删除空 204、导入预检 201、确认 202；真实 LAN Playwright 用 Bandizip XZ 包完成 11 文档/6 图片确认并断言 wire。首个细粒度错误 `docs.import.compression_unsupported(method)` 已从 typed cause 生成并替代通用 `invalid_input(detail)`。

TypeScript HTTP Runtime 现提供纯 `resolveFailureFeedback` Interface，把 remote/local failure 与 violations 投影为本地化主文案、恢复说明、字段错误、未映射摘要和仅供详情的 code/traceId；未知或无翻译失败使用调用动作提供的安全 fallback，21 tests 与 typecheck 通过。生成 TS 同时输出 code → message/recovery key metadata。Asset 的 22 个错误已迁入 catalog/生成代码，`resource_in_use` 任意 map 已收紧为 typed counts，Asset 全量 Go 通过。Identity 的 authorize/token operation 显式声明 `failureProtocol: oauth`，RFC 6749/OIDC error 不进入普通 Problem catalog；OIDC/Controller tests 通过。

Asset operation 现实际兑现资产读取 raw DTO、删除空 204、上传初始化 201、完成 201；全量 Go 通过。Workspace 按 source digest 替换 Identity/Asset Provider 后，Docs 真实 AE 双语导入再次通过，144 个节点与 2 份去重图片完成预检/确认，证明新 Asset 201、Docs 201/202/204、OAuth Provider 与现有 Consumer SDK/BFF 可共同工作；浏览器链路耗时约 2.7 分钟。

Foundation 发布候选已完整收口并提交为 `bc8a00b`：Go `httpcontract` 有 public-beta module metadata，候选 `go/v0.4.0`；JS bundle候选 `js-v0.7.0`，变化包为 content-nuxt 0.2.0、http-runtime 0.2.0、UI 0.3.0。Go race/vet/govulncheck、JS `verify:js`、release validator、全部公共 package pack 和 UI 独立 tarball consumer 全绿。尚未 tag 或发布。

## Next

Review 修复及 Identity、Commerce 推广已落地。Identity 删除 9 个过渡 override，并额外修正 GitHub 绑定历史与批量公开用户，11 个列表由真实 `items` collection/page DTO 驱动；Commerce 38/38 operation 完成 201/202、标准分页、恢复错误脱敏及真实 PostgreSQL HTTP 验收。下一步建立 Paste 单站 Work 并接入 Project v1；不创建新版本、标签或 Release。

## Progress

范围说明：发布、npm/OSV、Dependabot cooldown 与包版本处置归属 Workspace 的 Foundation 发布加固 Work；ContentEditor 跨 checkout 类型修复归属编辑器能力。它们因同一提交链与消费者验收出现在下方历史中，但不属于本 Work 的 Result 合同交付范围。

- 2026-09-04：建立统一错误合同 Work，确认先以一手规范和主流平台实践校准设计，再进入合同与迁移方案。
- 2026-09-04：完成主流错误实践调研，覆盖 HTTP/OAuth/GraphQL/gRPC、Stripe/GitHub、OpenTelemetry、重试幂等、前端无障碍、本地化、安全与生成治理；收敛 10 条 Foundation 设计约束。
- 2026-09-04：完成最小运行时、声明式 compiler、caller-first 三案比较；决定深化 HTTP Problem + 声明生成的混合 Interface，并把成功响应限定为 raw resource/page/cursor/operation/empty。
- 2026-09-04：实现 HTTP Result 双 Schema、Go validator/CLI、跨 manifest 引用校验和 Go catalog 生成；Docs 成为首个试点，生成代码替换手写 Descriptor map，并修复旧 catalog 漏项。
- 2026-09-04：补齐 compatibility diff、TS/i18n 生成与 freshness check；Docs 实际迁移 raw 201、空 204、预检 201、确认 202，并以真实 XZ 包完成浏览器验收；未知 ZIP method 首次采用 typed cause + 细粒度生成错误。
- 2026-09-04：实现 Foundation failure feedback resolver 与字段 violation projection；Asset 完成错误 catalog/生成代码和任意参数 map 收紧，Identity 用 operation contract 固定 OAuth 专用错误 Adapter。Foundation JS、Asset 全量 Go、Identity OIDC/Controller 门禁通过。
- 2026-09-04：Asset 兑现 raw get、空 204、上传 init/finalize 201；替换真实 Provider 后由 Docs 完成 2.7 分钟浏览器导入确认，验证 Identity OAuth、Asset 新状态和 Docs Result 合同组合兼容。
- 2026-09-04：完成 Go race/vet/govulncheck 与 JS lint/typecheck/unit/build；HTTP Runtime 升 0.2.0。确认 JS bundle 尚被两个既存格式项及 content-nuxt/ui 未升版阻断，需进入 Foundation 统一发布顺序处理。
- 2026-09-04：按实际公开变化将 content-nuxt/UI 升至 0.2.0/0.3.0，修复格式并更新 lock；`verify:js`、release validator、全包 pack、UI tarball consumer 全绿，形成 Go 0.4.0 + JS 0.7.0 候选。
- 2026-09-04：Foundation 候选提交为 `bc8a00b`；Docs `6d4da27`、Asset `3172282`、Identity `b04d2ea` 和 Workspace `3f14274` 已提交并从对应 committed revision 重建真实组合。
- 2026-09-04：首次发布标签暴露知识 Markdown 格式门禁，修复后形成 `go/v0.4.1` 与 `js-v0.7.1`；后者两次均仅因 npm audit API 超时或 503 中止。保留不可变标签，增加仅针对网络错误与 5xx 的有限审计重试，补丁候选推进为 JS 0.7.2（content-nuxt/http-runtime/UI 0.2.2/0.2.2/0.3.2）；漏洞结果仍立即失败。
- 2026-09-04：发布 `go/v0.4.1` 与 `js-v0.7.2`。npm audit API 在 CI 有限重试后仍持续超时；Dependabot 无开放告警，随后本地完整复跑 `verify:js`、release validator、全包 pack 与两组 Playwright conformance（3 + 12）并发布 7 个 tarball、manifest 和 SHA256SUMS。远端标签、7 项制品校验和及 Go module resolution 已复核。
- 2026-09-04：后续 main CI 证明 npm audit API 仍是持续单点故障；改用固定提交的 OSV Scanner 直接扫描 `pnpm-lock.yaml`。新门禁实际检出 Tiptap 1 个中危与 fast-uri 5 个高危，升级 Tiptap/Nuxt UI 并覆盖 fast-uri 后扫描为零；`verify:js` 与两组 Playwright conformance 复跑通过。本次只推进 main 与本地消费者，不创建新版本、标签或 Release。
- 2026-09-04：Dependabot 的四个 JS 更新任务因其强制 3 天 `minimumReleaseAge` 与 Nuxt UI 自动安装的最新 Tiptap peers 冲突而同时失败。保留全局成熟期，仅对必须锁步解析且受 OSV 门禁保护的 `@tiptap/*` 设置定向排除；精确复现命令 `pnpm install --lockfile-only --config.minimumReleaseAge=4320`、冻结安装、OSV 和 `verify:js` 均通过。
- 2026-09-04：Dependabot 重建继续检出发布仅 36 小时的间接依赖 `@iconify/collections@1.0.733`；不扩大排除范围，覆盖到已满足成熟期的 1.0.732。精确 Dependabot 安装命令、冻结安装和 OSV 复验通过。
- 2026-09-04：Identity 推广发现 RFC 7009 revoke 合法使用 `200 + empty`，end_session 同一端点可返回 302 redirect 或 204 empty。Operation v1 保留主 `success` 并新增可选 `additionalSuccesses`；仅 OAuth 允许 200 empty，普通 Problem API 仍强制 204。解析、Schema 与兼容性 diff 回归测试通过。
- 2026-09-04：暂停推广并完成双轴审查；明确把编辑器/依赖 cooldown 归回各自 Work，Asset/Identity 修复声明式 catalog 单一事实源、生成 freshness 与 compatibility diff，Identity 修复 Provider raw error，Asset Task 改 typed DTO。
- 2026-09-04：Docs 全量迁移证明各产品复制 `cmd/httpcontracts`、OpenAPI 投影和 coverage checker 会持续漂移；决定在本 Work 增加 Foundation 通用 project generator，各站仅保留声明式配置与业务 operation-error 数据。
- 2026-09-04：实现 Project v1 Go model、严格解析、OpenAPI operation 投影、stale route/catalog coverage、legacy projection 与 CLI producer/`-check` 流程；`httpcontract` 专项和 Foundation 普通全量测试通过，最终全量 race 尚待重跑。
- 2026-09-04：补齐 Project/Operation Errors JSON Schema、独立 errorsFile 严格解析与 CLI 端到端 generate/check 测试，确认临时 OpenAPI drift 检查和六类输出可由同一命令完成。
- 2026-09-04：增加从既有 operations manifest 机械提取 Operation Errors v1 的迁移入口；本地构建固定 CLI 后，Docs 70 operation generate/check 通过，证明消费者迁移不依赖先发布 Foundation。
- 2026-09-04：Docs 以 Project v1 完成 70/70 operation 与六类产物 generate/check；Workspace 管理、编辑器及 AE 双语导入 Playwright 全绿，验证 201/202/204 和安全错误反馈。
- 2026-09-04：Blog 68 operation 与 Asset 54 operation 通过本地 Project generate/check。Asset 证明部分 catalog code 只作为成功 DTO 内嵌 issue/errorCode；Project 增加显式 allowUnusedErrors，仍拒绝未声明豁免和真正 Problem 漏挂。
- 2026-09-04：Project Operation Errors 增加稳定 route→ID 与协议/成功形状 override，修复 Asset 初次迁移造成的 breaking ID/binary drift；Asset 最终仅有两条 errors behavioral change。Identity 80 条普通 API Project diff 为零，OAuth/OIDC 9 条保持独立 manifest。
- 2026-09-04：Shortlink 建立 8 个产品错误、24/24 operation 与统一反馈，移除 SDK/BFF raw provider body，CI 改用 OSV；公开页 6 项和完整登录/创建/302/编辑/治理/410 flow 通过，真实断言举报 201 与删除 204，功能分支 fast-forward 合并本地 main。
- 2026-09-05：Project producer 支持声明非敏感静态 env，Notification 使用 config.example 成功生成并检查 16 个错误和 24/24 operation；敏感值继续禁止进入 project config。
- 2026-09-05：Notification 完成状态、管理 DTO、CI 与真实 HTTP integration；管理查询不含 Provider lastError。后续 review 将发送统一收敛为 200，并保留 Replay 的标准 202 OperationDTO。仓库无独立 Workspace target，因此不虚构组合验收。
- 2026-09-05：双轴 review 发现 Notification 滥用 additionalSuccesses/非标准 OperationDTO、Project 放行 list/entries 和不完整分页。已修复生成器与 Notification；6 个消费者新版 check 中 Blog/Asset/Shortlink 直接通过，Docs 修正模型后通过，Identity 以 9 个显式 transitional overrides 保持 diff 为零。
- 2026-09-05：Commerce 完成 38/38 Project v1、12 个业务错误、31 个 200/5 个 201/2 个 202、6 个分页与 4 个集合；移除恢复 `lastError`，真实 PostgreSQL HTTP integration、race/vet/govuln 通过，单站 Work Finished。
- 2026-09-05：Identity 将 9 个过渡 override 全部迁为真实 DTO，并补迁 GitHub 绑定历史与批量公开用户；Account/Nuxt 消费者、Go HTTP e2e、全量门禁及 CLI Playwright 通过，单服务 Work Finished。兼容性 CI 改为比较最近正式发布 tag，而非把尚未发布的 main 提交误作公开基线。

## References

- [主流前后端错误合同调研](references/mainstream-error-practices.md)
- [HTTP Result Contract 设计](references/http-result-contract-design.md)
- [HTTP Result 知识](../../knowledge/errors/http-result-contract.md)
- [错误目录知识](../../knowledge/errors/error-catalog.md)
- [失败反馈知识](../../knowledge/errors/failure-feedback.md)
