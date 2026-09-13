# 通用移动底部导航

## Goal

从已验证 BVideo 样稿提取产品无关 MobileBottomNav，接通消费者并提交本地候选。

## Status

Finished

## Current

已导出 @yueli/ui/navigation/mobile-bottom-nav，Nuxt 自动注册 YMobileBottomNav。组件负责安全区、SSR占位、紧凑导航、操作入口、徽标和输入隐藏；产品保留路由匹配、权限和业务动作。Toast直接消费共享占位变量，BackToTop通过data-y-dock发现导航。BVideo已删除本地组件和通用CSS，使用正式公共入口。

## Next

None。兼容新增并入未发布0.4.0候选，不包含远端发布。

## Verification

- UI 104项单测、vue-tsc、受影响ESLint/Prettier与diff检查通过。
- test:pack验证最终文件与公开exports，位于系统临时目录的tarball-only消费者独立安装、typecheck和生产build通过；不依赖相邻Workspace overlay。日志：js/test-results/mobile-nav-pack.log。
- 仓库内CLI Playwright conformance在真实BVideo上验证390/750/768/1440px、SSR占位、输入隐藏、桌面释放和真实Toast region的65px/16px回退，通过。
- BVideo接入后16项UX/密度用例通过，6项按平台分工跳过；类型检查、生产build通过。业务验收由BVideo Work拥有。
- 截图确认样稿外观延续。实体手机安全区与软键盘未在本环境连接硬件验收。

## References

- [Context](context.md)
- [包说明](../../../js/packages/ui/README.md)
