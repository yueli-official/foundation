# 2026-09-14 统一后台发布验收

用户授权 Windows 更新后快速 review、测试、提交并发布 Yotta 服务器。发布已完成。

- Foundation：`7bad41a86802ac2f6cf3de87bbc9e6258b84435e`；Go `go/v0.5.0`、JS `js-v0.8.0`。CI 34858233785、JS Release 34858233813 成功。
- 正式 UI 0.5.0 SHA256：`1efd69d0ccb06cf47bb76ce5a2b0376b3b91b3e9173062789637db98c1edd6a4`。该值替代发布前候选摘要。
- Registry：`eee2452fd9eafd36acc49fd957e9a2a1f26634e9`，正式 Go 0.5.0 import，无内部 classificationwire、replace 或跨仓源码依赖。
- Hub：`bd389ccc614883a7011535bfd2c047c03f2e2eec`，正式 GitHub Release UI tarball 与 lockfile；产品产物 dirty=false。
- Yotta：`b835686f`，市场发布条款清理；task check 168 文件/713 前端测试通过。产品代码均已 push；本次未重新发布桌面安装包或重启 App。

双路 review 修复：Foundation 子导航 search:false 被忽略；Hub 设置 query 栏目切换丢草稿；Registry 分类保存后重新查询存在并发删除窗口。分别有红绿/事务/真实浏览器回归验证。

验证：Foundation 完整 Go/JS 与制品消费门禁、远端 CI/Release；Registry task check、race/vet/contracts 与真实 PostgreSQL 集成；Hub Go race/vet/contracts、正式 UI typecheck/build、Workspace CLI Playwright web 全流程；设置栏目切换单独真实浏览器验证取消保留/确认离开/无设置写入；部署工具 7 测试通过。

线上 https://yotta.yuelili.com 已 Plan/Apply/Status 通过，3 容器健康，6 公网检查通过。Deployment 提交 `0d2a71a`（该仓没有远端）。

| 组件     | 镜像                                 | 不可变 image ID                                                         |
| -------- | ------------------------------------ | ----------------------------------------------------------------------- |
| Registry | yotta/registry:test-9c7d2538919b1433 | sha256:2699f3153da04c95a9710400df2cf1b19bdfd1a0e3f4e36c8ba967d2625d9fa6 |
| Hub API  | yotta/hub:test-fde3081e306177fa      | sha256:0e2674fa75bd7acfebb8bbe8ee66f0d4faaecf66a79812311b582b9e72b98866 |
| Web      | yotta/hub-web:site-0a0f1db804f7b1fe  | sha256:6da73973c6fa3accb4848b4acf3ac98c159a780b8725d1ddeda4989365a8c529 |

服务器数据库备份：`/projects/yottaapp/deployment/history/pre-admin-20260914T150423Z`；部署回滚快照 `20260914T150618910555Z`。数据库、密钥、Registry 制品保留；source.json 已记录实际已部署产品提交。

证据：各产品 `.task/admin-release-*`；Workspace `.task/admin-release-browser.log`；Hub `.task/site-settings-query-live/result.json`；Deployment `.task/admin-release-{plan,apply,status}.log`。

剩余范围：BVideo/Blog 试点本地验证已完成，但尚未在本次任务切换正式制品或上线；端到端新产品生成器尚未实现。不能将本次 Yotta 上线描述为所有站点与快速创建功能均已完成。
