# HTTP Result Contract

公开网站 API 使用 HTTP 自身表达成功或失败，不增加 `{code,data,message}` 通用 Envelope。成功响应直接返回端点 DTO；失败响应使用 Foundation HTTP Problem v1。`items` 是集合 DTO 字段，不是响应 Envelope。

## 成功响应

- 单资源读取：`200 + ResourceDTO`。
- 有明确小规模上限、不需要分页的集合：`200 + {items}`；空集合编码为 `[]`，不使用 `null`。
- 页码分页：`200 + {items,page,size,total}`。
- 游标分页：`200 + {items,nextCursor?}`；不混入页码字段，也不增加与 `nextCursor` 重复的 `hasMore`。
- 创建：`201 + ResourceDTO`，能提供稳定地址时同时返回 `Location`。
- 异步受理：`202 + OperationDTO`；Operation 自己拥有稳定 `id/status`。
- 成功且无正文：`204`，不得再写 `{deleted:true}`、空对象或其他 JSON。
- 文件、流、条件请求与重定向使用专用 Adapter，不套 JSON Envelope。

产品 DTO schema 仍由产品和 OpenAPI 拥有。Foundation operation declaration 只记录 method、path、成功 status/body kind、DTO schema 引用和可能的公开错误，不复制 DTO 字段。

## 失败响应

普通 JSON API 返回 `application/problem+json`：

```json
{
  "type": "https://errors.yueli.dev/problems/docs.import.compression_unsupported",
  "status": 400,
  "code": "docs.import.compression_unsupported",
  "params": { "method": 99 },
  "violations": [],
  "traceId": "..."
}
```

- HTTP status 与 body `status` 必须相同。
- `code` 和参数供程序分支；客户端不得解析 `title`、`detail` 或内部错误字符串。
- `traceId` 关联一次 HTTP/分布式执行；导入批次、订单、Run 等长期事实继续使用自己的业务 ID。
- OAuth/OIDC endpoint 遵守 RFC 6749/OIDC 的 `error` wire format，通过专用 Adapter 投影，不强制改成 Problem。

依据：[RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html)、[RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html)、[RFC 6749](https://www.rfc-editor.org/rfc/rfc6749.html)。

