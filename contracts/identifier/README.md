# Identifier contract

此目录定义跨语言稳定的 Identifier wire contract。`profiles.v1.json` 是 Public Locator Profile v1 的机器可读清单，
`derive.v1.json` 是确定性 UUIDv5 的跨语言向量；Go 实现位于 `go/identifier`，JavaScript/TypeScript 实现位于
`js/packages/identifier`。随机输出不能使用 golden value 验证，各语言应验证格式、解析、错误和分配状态机。

UUID writer 使用 RFC 9562 UUIDv7，wire 形式为规范小写 36 字符。确定性 writer 使用 UUIDv5；拥有领域必须另外发布
namespace、canonical name 字节顺序和版本。

Public Locator 可安全展示和记录，但绝不表达授权。需要唯一性的值只有在产品数据库的具名 `UNIQUE` 约束原子 claim
成功后才算完成分配。
