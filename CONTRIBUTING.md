# Contributing

## Module bar

新增公开 Module 必须：

1. 通过 deletion test，并以小 Interface 隐藏真实复杂度；
2. 说明责任、非责任、runtime、错误、安全和兼容性；
3. 不依赖 platform workspace alias、内部域名、凭据或产品 DTO；
4. 通过 unit/typecheck/build，并由 packed-artifact conformance consumer 验证；
5. 在 maturity manifest 中保持 `experimental`，直到真实消费者与文档门禁完成。

JS 和 Go 共享 repository，但分别执行测试与发布。跨语言 contract 变化必须同时更新 schema、fixtures 和两侧 conformance。
