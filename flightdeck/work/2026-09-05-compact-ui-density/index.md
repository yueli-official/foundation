# 可选紧凑 UI 密度

## Goal

提供产品无关、显式启用的compact密度，统一共享表单、评论、后台标题、导航与行操作，默认消费者不改变。

## Status

Finished

## Current

已通过SSR文档根属性data-yueli-density="compact"启用共享主题变量，Portal自然继承。PageHeader、AdminConsoleLayout、AdminRowActions、AccountMenu、PublicCommentThread/Composer通过变量消费尺寸；既有无属性消费者保留原回退值。表单语义类统一14px，不在产品复制共享组件。

UI兼容新增升为0.4.0候选，README提供启用与消费者迁移说明。BVideo已完成真实接入与前台/后台密度回归。

## Verification

- UI单测102/102通过；后续评论最小高度修正的5项定向单测再次通过。
- UI vue-tsc、受影响源码ESLint、Prettier和diff检查通过。
- 仓库内CLI Playwright compact-density conformance在现有消费者上通过：390/750/1440中共享评论/字段紧凑值与移除属性后的原默认值均符合合同；未新建测试服务。
- 最终tarball-only Nuxt消费者无相邻源码overlay，独立安装、typecheck、production build通过。示例使用共享ManagePage、AdminRowActions、PublicCommentThread及Nuxt字段。
- tarball SHA256：27A081D43E1E7AB85C88A1328968D0A20F168155635FAEDAC8D43ABD21E9072C。
- BVideo九档前台与登录态后台矩阵、真实业务路径和Axe已通过，具体证据由产品Work持有。

## Delivery boundary

已形成可审查本地候选，未push/tag/release。用户此前提交授权用于本次相关实现；只提交本Work拥有的文件，保留原有未跟踪workspace/目录。未把本地包交付记为正式发布，也未运行远程CI。

## Next

None

## References

- [上下文](context.md)
- [包说明](../../../js/packages/ui/README.md)
- [浏览器合同](../../../js/conformance/ui/test/e2e/compact-density.spec.ts)
- [BVideo Work](../../../../bvideo/flightdeck/work/2026-09-05-compact-ui/index.md)
- 忽略的制品与消费者：js/test-results/compact-density-pack/、compact-density-consumer/。
