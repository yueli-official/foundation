# 上下文

用户授权先写计划再执行，首批 Identity、Foundation 与 Blog。不是全站推广 Work，不修改 Workspace 合同或顶层 Flightdeck。不提交已有无关工作树改动。

令牌绑定用户、具体站点实例、明确能力与有效期。业务请求取令牌范围与用户当前权限交集，再检查资源归属。禁止以创建时角色快照永久授权，新增角色能力不自动扩张令牌。账号安全与令牌签发入口不向 PAT 开放。

消费者只公开显式可委托能力；创建 UI 只展示当前用户可授权能力，提交由后端再次校验。Identity 不存消费站点角色或资源 ACL。原业务接口和函数复用。

官方依据：
- https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens
- https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens
- https://docs.gitlab.com/security/tokens/access_token_scopes/

现有仓库有大量已授权历史改动，必须保留。所有服务生命周期通过 Workspace CLI，验收用隔离组合，避免影响已运行站点。

用户明确要求令牌支持再次查看和复制，不接受永久只显示一次。原文必须加密保存并限本人账户会话显式读取，列表不返回原文。已有仅摘要记录无法还原；不自动重置用户令牌。
