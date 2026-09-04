# HTTP Result Contract 设计

## 决策

Foundation 直接深化现有 HTTP Problem，不新增跨所有协议传播的 `Result<T>` 或通用成功 Envelope。普通网站只使用 HTTP JSON；OAuth/OIDC 由专用 Adapter 遵守其协议格式。成功响应继续是原始 DTO，失败响应继续是 RFC 9457 Problem v1。

运行时 Interface 保持小：产品用生成的 typed constructor 建立公开错误，HTTP Adapter 负责 status、content type、trace、参数校验、安全降级和日志；普通 200 由框架直接编码，只有 201、202、204 使用显式 helper。构建时 Interface 为 `compile / diff / verify`，从产品拥有的声明式 catalog 生成 Go、TypeScript、OpenAPI、i18n inventory、文档和 conformance fixture。

## 成功响应模型

- 单资源读取：`200 + ResourceDTO`。
- 有明确小规模上限且无需分页的集合：`200 + {items}`。
- 集合页码分页：`200 + {items, page, size, total}`。
- 集合游标分页：`200 + {items, nextCursor?}`；不同时返回页码字段，不增加与 `nextCursor` 重复的 `hasMore`。
- 创建：`201 + ResourceDTO`，可同时提供 `Location`。
- 异步受理：`202 + OperationDTO`；Operation 必须有稳定 `id/status`，业务 ID 不塞入通用错误结构。
- 成功但无正文：`204`，不得发送 JSON body。
- 文件、流、重定向使用专用 Adapter。
- 禁止新接口产生 `{code,data,message}` 成功 Envelope；`items` 是集合 DTO 字段，不是响应 Envelope。

## 失败响应模型

HTTP wire 继续使用：

```text
Problem {
  type
  status
  code
  params?
  violations?
  traceId
}
```

`category`、默认 retry policy、message key 和 recovery key 属于 catalog 元数据，不在每次响应中重复。`operationId` 只由跨请求的真实业务 Operation 持有；`batchId/orderId/runId` 是业务 DTO 字段。内部 cause 只供日志和 tracing，不进入 Problem。

## 所有权

- Foundation：Schema、compiler、compatibility diff、Go/TS runtime、HTTP/OAuth Adapter、公共 code、conformance。
- 产品：业务 code、参数、cause 映射 seam、翻译、恢复动作、DTO schema。
- UI：把 violation 映射到字段，并按动作选择 inline/Alert/Toast；机器码只进入详情，不作主文案。

## 三种候选比较

最小 Writer 方案的运行时 Interface 最小，但单独采用无法解决 catalog、翻译、OpenAPI 和多仓漂移。纯声明式 compiler 的 locality 和生成 leverage 最高，但若同时描述完整 DTO 会成为第二套类型系统。caller-first 方案最适合 GoFrame/Nuxt 调用，却仍需要构建时事实源。

最终采用混合：声明只拥有错误及 operation 的成功形状/状态，DTO 字段继续引用现有 OpenAPI schema；运行时复用现有 Problem Module，普通成功路径不穿过新包装层。

## 兼容策略

- HTTP Problem v1 wire 不变，新增 catalog 是约束加强而非响应格式升级。
- 已发布成功响应不原地移除 wrapper；通过版本化端点迁移。开发期产品可直接收敛。
- Legacy Envelope 只在客户端保留有期限的兼容 Adapter，新服务端输出由门禁禁止。
- catalog 删除/改名 code、修改 status、收紧参数或改变成功 body kind 属于 breaking change。
