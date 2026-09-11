# 编辑器图片输入与文档导入

## Goal
共享 ContentEditor 支持图片选择/拖拽/粘贴，以及 Markdown、HTML、Word 导入并归档正文图片。

## Status
Finished

## Current
共享图片对话框支持多文件选择、拖拽、粘贴及原位进度/失败反馈；正文支持图片粘贴和拖拽，串行处理所有文件，异步插入位置随编辑器事务映射。导入按钮支持 Markdown、HTML、Word docx、带相对路径资源的 ZIP 及富文本剪贴板。复用产品 imageUploader 和 Asset 处理流程，成功后一次插入，缺图或失败不改原文；重复图片引用只上传一次，失败重试复用已完成的导入图片。

WWW、Blog、Docs 前端已更新为 server-20260909-import-1，后端和数据库未改动。候选包仅更新本次共享编辑器文件与导入依赖，未正式发布 Foundation NPM Release，未提交 Git。Gallery 保持本地。

## Next
None

## Verification
- 14 项共享内容测试通过，包含 Word 内嵌图、ZIP 相对路径、缺图预检、重复引用和重试；共享类型检查通过。
- WWW 本地真实服务 CLI Playwright：390/1440，多文件上传框、正文拖拽/粘贴、对话框拖拽/粘贴、Markdown/HTML/Word/ZIP 导入、富文本粘贴、缺图保持原文、保存重载通过。测试内容已删除。
- WWW、Blog、Docs 独立候选类型检查、生产构建成功。线上三站 390/1440 公开页返回 200，无溢出或 pageerror；正式编辑器 JS 资源返回 200 并包含新入口。没有把线上静态资源/公开页检查冒充真实管理员上传验收。
- 外链图片经浏览器读取，来源不允许 CORS 或请求失败时明确提示补充本地文件，不静默丢图；Word 保留编辑器支持的语义格式。图片仍由产品原有处理器负责裁剪/转换/上传，不新增服务器远程抓取接口。
- 新增依赖的锁文件保留原有条目，frozen/offline 安装验证通过；共享层统一 ProseMirror 去重，解决新输入模块与已有编辑器运行时重复实例。

## References
[背景](context.md) · [主要浏览器验收](references/browser.json) · [Word 与剪贴板](references/browser-extra.json) · [线上复查](references/production.json) · [源文件](references/source-hashes.json)。完整脚本、构建和部署日志：E:/tmp/yueli-editor-import-20260909。

当前追加：URL 图片获取失败以带原图链接的占位文字替代，不阻断其他内容。已验证部署。

## 外链图片失败继续导入（2026-09-09）
HTTP(S) 图片仍先尝试下载；读取或上传失败时插入“图片未导入”与原图链接，继续正文和其余图片。剪贴板图片文件/Base64 处理不变；本地缺图仍提示补齐。占位使用可持久化文本与链接，避免破损 img 和自建节点。
共享测试 16 项及 typecheck 通过；WWW/Blog/Docs 候选 typecheck/build 全通过。本地 Workspace WWW Playwright 验证 Markdown、剪贴板 HTML 混合失败 URL 和正常图片：真实尝试失败 URL 两次，正常图片经 Asset 上传，原文/导入首尾保留，保存回读两个占位链接与两张正常图片。
线上三站 Web 更新为 server-20260909-placeholder-1，均 healthy。WWW、cg.yuelili.com（Blog）、Docs 390/1440 首页及对应新编辑器 chunk 200，无 pageerror。线上未使用真实管理员验证上传；完整交互验收为本地真实依赖组合。WWW API 仍为 download-1，版本与下载接口不变。
制品、脚本与日志：E:/tmp/yueli-placeholder-20260909；远端 /projects/yuelili.com/.deploy/editor-placeholder-20260909。本次 WWW 临时组合已停止，未提交或推送 Git。
