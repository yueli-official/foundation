# 编辑器命令栏与集合编辑入口

## Goal

统一 Blog 模板的编辑命令与 Docs 风格的集合编辑入口，保留产品发布和权限语义。

## Status

Finished

## Outcome

共享 EditorCommandBar 与 isCollectionEditGesture/CollectionPanel editItem 已交付。Blog、Docs、WWW、Resource、Shop 接入，所有正文编辑器覆盖；全局设置文本保留设置保存语义。公开/下架是实际匿名读取边界，不引入仅链接可见。

## Evidence

16 项相关组件/交互测试和 Foundation 类型检查通过。五站通过 CLI Playwright 四种宽度、浅深色及编辑/发布交互；WWW 单独覆盖发布失败与快速编辑字段保留；Docs 覆盖新建直接发布与树刷新。五站类型检查与构建通过。Blog/Docs/WWW 独立固定候选 editor.1 部署并健康，线上公开页 390/1440px 复查通过；线上管理员编辑交互由本地等价验收覆盖，不宣称进行了生产写入测试。

详细源文件校验、浏览器证据见各产品 2026-09-08-editor-workbench。共享规则见 [编辑工作台知识](../../knowledge/frontend/editor-workbench.md)。

## Remaining

无。公开包发布不属于本次请求；现有线上使用固定独立候选制品，其他站保留本地实现。

编辑工作台规则已补齐：正文直接写作，slug/摘要等元信息默认折叠在 Inspector；固定发布入口，加载失败禁用写入并反馈。WWW/Docs 修正版的本地交互、独立构建及在线公开页面复查通过，见各产品编辑工作台 Work。
