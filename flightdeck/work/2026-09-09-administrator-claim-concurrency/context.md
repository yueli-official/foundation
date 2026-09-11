# Context

来源：Paste 本地验收接入首位管理员认领。仅修复 projection 重建的事务竞争，不变更授权定义或认领规则。
代码：go/authorization/postgres/store.go；共享集成测试由两个并发请求扩展为两个 Adapter/20 请求。
证据：Paste flightdeck/work/2026-09-05-http-result/references/fixes-20260910.md；测试使用独立 schema 并清理。
本次不发布包，消费者只通过 Workspace local Foundation overlay 使用修复。
