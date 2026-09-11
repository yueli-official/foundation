# 管理后台图标选择

复用 `AdminIconPicker`。`options` 提供产品常用图标及中文名称，不应隐式禁用完整 Tabler 搜索；只有业务明确限制范围时才设置 `fullCatalog: false`。完整目录按选择器挂载时懒加载，搜索结果有数量上限，不把全库打入首页入口。

图库之外的持久化选择必须能在新浏览器显示。沿用共享 UI 的同源 Nuxt Icon server provider 和内置 Tabler server bundle，外部 Iconify fallback 保持关闭；产品不得再覆盖成 provider none。常用 UI 图标仍通过构建期 client bundle 首帧交付。

验收同时覆盖：带 options 搜索非常用图标、选择保存后重载、所有搜索结果图形可绘制，以及匿名公开页面。仅断言按钮存在不足以证明图标渲染成功。
