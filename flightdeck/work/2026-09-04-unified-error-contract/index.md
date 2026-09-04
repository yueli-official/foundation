# 统一 HTTP 结果合同

## Goal

交付一个由 Foundation 拥有的深 HTTP Result Module：统一成功响应模型、跨语言失败合同、产品错误目录生成、传输降级、前端反馈与治理门禁，同时保留产品对 DTO、业务错误和恢复动作的所有权。

## Status

Open

## Current

Foundation 已有 HTTP Problem v1、Go Problem/GoFrame/HTTP Client 与 TypeScript HTTP Runtime，但产品错误目录、公共 taxonomy、重试语义、Operation 关联、i18n 完整性和 UI 降级仍分散。Yotta 已验证 canonical envelope、单次语义投影、operationId、retryable、durable identity、transport violation 和 i18n inventory 的价值，但其 Wails 模型不能直接替代 Web Problem 合同。

主流一手资料调研已完成。RFC 9457、OAuth 2、GraphQL、gRPC/Google AIP-193、Stripe、GitHub、OpenTelemetry 与 W3C 的共同方向是：协议状态、稳定机器原因、类型化上下文、最终用户文案和内部 cause 必须分离；重试需要同时考虑幂等性、执行状态和服务端 hint；字段违规需要可定位；trace、operation 与 durable identity 必须各自定义生命周期。研究已形成 canonical failure 候选模型、10 条设计约束和首阶段范围。

Interface 已完成三案比较并定稿：不引入 `Result<T>` 或 `{code,data,message}` 成功 Envelope；普通成功返回原始 DTO，集合统一 `items`，201/202/204 显式表达；失败直接深化现有 HTTP Problem v1。构建时以产品声明式 catalog 生成多语言产物和门禁，运行时由现有 Adapter 隐藏 status、trace、校验与安全降级。

第一段实现已落地：`contracts/http-result` 定义错误目录与 operation 成功形状 Schema；Go `httpcontract` 提供严格解析、namespace/status/参数/violations/成功形状校验、跨 manifest 引用检查、CLI 和确定性 Go catalog 生成。支持 raw resource、`{items}`、页码/游标集合、201 创建、202 operation、204 empty、binary 与 redirect，并编码互斥规则。Docs 试点 manifest 已通过 validator，其手写 code/status map 已由生成文件替换；过程中发现并补回旧公开 catalog 遗漏的 `docs.administrator_grant_protected`。

构建时 Interface 已补齐 compatibility diff、TypeScript discriminated union、i18n inventory 和 committed-output `-check`；Foundation Go 全量与 HTTP Runtime 19 tests 通过。Docs 试点已实际兑现集合创建 raw DTO + 201、删除空 204、导入预检 201、确认 202；真实 LAN Playwright 用 Bandizip XZ 包完成 11 文档/6 图片确认并断言 wire。首个细粒度错误 `docs.import.compression_unsupported(method)` 已从 typed cause 生成并替代通用 `invalid_input(detail)`。

TypeScript HTTP Runtime 现提供纯 `resolveFailureFeedback` Interface，把 remote/local failure 与 violations 投影为本地化主文案、恢复说明、字段错误、未映射摘要和仅供详情的 code/traceId；未知或无翻译失败使用调用动作提供的安全 fallback，21 tests 与 typecheck 通过。生成 TS 同时输出 code → message/recovery key metadata。Asset 的 22 个错误已迁入 catalog/生成代码，`resource_in_use` 任意 map 已收紧为 typed counts，Asset 全量 Go 通过。Identity 的 authorize/token operation 显式声明 `failureProtocol: oauth`，RFC 6749/OIDC error 不进入普通 Problem catalog；OIDC/Controller tests 通过。

Asset operation 现实际兑现资产读取 raw DTO、删除空 204、上传初始化 201、完成 201；全量 Go 通过。Workspace 按 source digest 替换 Identity/Asset Provider 后，Docs 真实 AE 双语导入再次通过，144 个节点与 2 份去重图片完成预检/确认，证明新 Asset 201、Docs 201/202/204、OAuth Provider 与现有 Consumer SDK/BFF 可共同工作；浏览器链路耗时约 2.7 分钟。

Foundation 发布候选已完整收口并提交为 `bc8a00b`：Go `httpcontract` 有 public-beta module metadata，候选 `go/v0.4.0`；JS bundle候选 `js-v0.7.0`，变化包为 content-nuxt 0.2.0、http-runtime 0.2.0、UI 0.3.0。Go race/vet/govulncheck、JS `verify:js`、release validator、全部公共 package pack 和 UI 独立 tarball consumer 全绿。尚未 tag 或发布。

## Next

取得最终发布确认后由发布加固 Work 创建 `go/v0.4.0` 与 `js-v0.7.0`。发布后更新 Docs/Asset 锁定依赖并消费 `resolveFailureFeedback`。

## Progress

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

## References

- [主流前后端错误合同调研](references/mainstream-error-practices.md)
- [HTTP Result Contract 设计](references/http-result-contract-design.md)
- [HTTP Result 知识](../../knowledge/errors/http-result-contract.md)
- [错误目录知识](../../knowledge/errors/error-catalog.md)
- [失败反馈知识](../../knowledge/errors/failure-feedback.md)
