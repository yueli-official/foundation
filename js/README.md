# JavaScript and TypeScript modules

- `packages/` 包含独立发布的 `@yueli/*` packages。
- `conformance/` 包含从 packed artifact 安装并验证公开 Interface 的最小消费者。
- `apps/` 只包含文档和产品中立的开发工具。

当前 `apps/ui-lab` 是从旧 platform Preview 筛选迁移出的公共 Pattern 实验室，只消费 `@yueli/*` exports；它不是公共组件实现源码。

Package 必须使用显式 exports，禁止依赖 platform workspace alias 或内部部署拓扑。
