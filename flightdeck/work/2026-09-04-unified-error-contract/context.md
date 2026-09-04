# Context

## Existing foundation

- `contracts/http-problem` 定义跨语言 HTTP Problem v1，要求 `code/status/traceId`，支持 `params/violations`。
- Go `problem`、GoFrame Adapter、Go HTTP Client 与 TypeScript HTTP Runtime 已实现编码、解码、验证和安全限制。
- 产品目前各自维护 Descriptor、错误目录生成、翻译和部分 Legacy/BFF Adapter，公共错误语义和 UI 恢复体验仍有漂移。

## Design constraints

- Foundation 只拥有可独立发布的公共机制与公共语义，不吸收产品业务错误。
- 产品继续拥有业务错误 ID、稳定参数、语义映射和真实恢复动作。
- 原始 cause、SQL、私有路径、凭据和 Provider body 不进入公开失败参数。
- HTTP trace、跨请求 operation 和持久业务 identity 是不同概念，不互相替代。
- 新合同必须有明确版本策略、消费者迁移路径和跨语言一致性门禁。
- 当前网站不使用 GraphQL/gRPC；它们只作为调研对照，不进入首阶段实现。
- 成功响应不采用 `{code,data,message}` Envelope；`items` 是集合 DTO 字段。
- 普通成功直接返回 DTO；统一页码分页、游标分页、201、202、204 的状态与形状。

## Reference system

Yotta 的错误链路提供可借鉴的行为：domain cause 在语义 seam 映射一次；transport 只归一化；错误携带 category、retryable、operationId/runId；异步失败持久化；i18n inventory 阻止遗漏。其 Wails Envelope、`Params any` 和仓内源码扫描不作为站群公共合同直接复用。
