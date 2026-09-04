# Error Catalog

每个产品维护一份声明式公开错误目录作为事实源。Foundation 拥有 Schema、验证器、兼容性 diff 和生成器；产品拥有业务 code、稳定参数、domain cause 映射和恢复语义。Foundation catalog 只收录在所有产品中语义、参数和恢复方式都一致的 `common.*` 错误。

## 错误定义

每个错误声明：

- namespaced stable `code`；
- 唯一 HTTP status；
- message key 和可选 recovery key；
- 每个公开参数的类型、必填性和预算；
- 是否禁止、允许或要求字段 violations；
- 迁移期需要保持的生成语言名称。

业务 code 应描述稳定用户语义，例如 `docs.import.compression_unsupported`，不得使用函数名、DAO、Provider 或临时实现。不要把不同恢复动作都压成 `invalid_input` 或 `failed`。

公开参数只允许有界 string、bool、有限 number 或这些标量的短数组。禁止传入任意 map、`err.Error()`、SQL、堆栈、Token、Cookie、Provider body、本机绝对路径、对象存储内部 Key 或凭据。字段错误使用 RFC 6901 pointer + 稳定 violation code；字段标签和文案由 UI 本地化。

## 映射 seam

错误只在最了解业务语义的 seam 映射一次：

1. Domain/Adapter 返回 typed cause，并保留 `errors.Is/As`。
2. 产品 application/service 将 typed cause 映射成 catalog error。
3. HTTP/OAuth Adapter 只负责协议投影、trace、status、header、编码和安全降级，不再猜业务 code。
4. 未知 cause 只返回安全内部错误；原始 cause 进入受控日志和 tracing。

生成器从 catalog 产生 Go Descriptor/常量、TypeScript discriminated union、message/recovery metadata、i18n inventory 和公开目录。CI 使用 `-check` 阻止生成物漂移，并对前后 catalog/operation 执行 compatibility diff。

删除或改名 code、修改 status、删除/收紧参数、把可选参数改为必填、改变 violation policy 或 operation 成功 status/body kind 都是 breaking change。新增 code 和可选参数通常是 additive；改变 message/recovery key 是 behavioral change。
