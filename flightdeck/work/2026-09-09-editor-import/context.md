# 背景

用户偏好紧凑、克制、少用色的共享后台组件。编辑器使用 Nuxt UI UEditor + Tiptap Markdown，持久化 Markdown，产品注入 imageUploader(File) 返回 Asset 稳定媒体 URL。

导入在处理成功后插入光标位置，不能失败后留下半篇内容或静默丢图。Markdown 引用本地图片可补充多文件或 ZIP；远程图片能读取时转存，不能读取时明确指出并允许补充。用户已询问富文本格式，默认准备 HTML 与 Word docx。

本地验收通过 Workspace CLI，不能手写生成状态。Gallery 保持本地。各站已有业务逻辑保持不变。
