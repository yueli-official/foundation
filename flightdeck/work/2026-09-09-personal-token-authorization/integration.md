# 个人令牌接入合同（本地验收通过）

Identity 只保存凭证和明确 scopes，消费者保存当前角色与资源权限。生效权限是 Token scope 与当前业务授权的交集，目录展示不能代替资源决策。共享授权的 IncludeDescendants 仅用于发现潜在能力，不会授予兄弟资源的访问权。

## 配置

Identity `pat.applications` 是部署管理的数组：`id` 必须为已有 OIDC client ID；`name` 为显示名；`permissionsUrl` 为消费者的可信能力查询端点；`audience` 为该消费者 JWT audience。首个实例 id/audience 均为 `blog-main-web`。禁止从用户输入解析注册目标。

Blog `blog.personalTokens.siteId=blog-main-web`，且必须匹配自身 audience。在线验证端点由既有 `blog.identity.baseUrl` 拼接 `/api/v1/pat/verify`。

Asset `asset.personalTokens.authorities` 映射实例 ID 到可信消费者 `/api/v1/personal-token/media-authorization` URL。Blog 只派生 `asset.profile.blog-post.upload`，Asset 仍验证原有 client/namespace/profile 绑定和文件所有者，拒绝其他管理入口及 multipart。

默认 HTTPS，回环 HTTP 可用；明确受信私网才使用 Identity `pat.allowHTTP`、Blog `blog.personalTokens.allowHTTP`、Asset `asset.personalTokens.allowHTTP`。均禁止重定向转发凭证。

## 请求与权限

客户端使用 `Authorization: Bearer <PAT>` 调用原有 Blog API。共享 Nuxt BFF 保留显式 PAT，不换成浏览器 Cookie 身份。公开未发布内容不会因此绕过现有可见性规则。

首批允许读取自己的管理列表、创建、编辑、发布、下架文章及上传文章图片。权限目录只列消费者明确声明的可委托能力；未审查路由默认拒绝。PAT 不可进入账号安全、令牌签发或站点治理入口。

共享 `auth.PersonalScope` 编码站点实例与单个能力，不使用角色或通配符。Identity 展示目录和提交创建时分别查询当前权利；目录查询最多四路并发且有总超时，失联站点不可授权。沿用 scopes JSON。0035_pat_recovery 新增 token_ciphertext，用于加密恢复；旧记录默认空串，不改变其验证能力。

每次请求在线验证，无有效 Token 正缓存。撤销在下次验证生效，已经授权执行中的请求不会被中途取消。产品角色仍由 Foundation 授权模块判断；当前持久化适配器有进程内投影，单实例进程模型有效，不能声称支持多进程即时失效。

## 发布前未完成项

真实 PostgreSQL/Asset 组合验收已通过。正式 Foundation Go 与 Identity Nuxt 制品更新及消费者依赖验证尚未完成；本地源码通过不代表已上线。生产部署需三方可信 URL/实例配置一致。不得直接以本地 overlay 代替发布依赖。

## 本人再次复制令牌

POST `/api/v1/pat/{id}/reveal` 仅接受有效本人账户会话，拒绝 PAT 替代登录；查找范围限定 owner，过期和撤销不可读取。返回 `{token}`，响应 `Cache-Control: no-store`，创建响应同样禁缓存。审计只记录 Token ID，不记录原文或密文；普通列表仅返回 recoverable 标记。

新 Token 保留现有 HMAC 用于验证，额外使用 AES-GCM 随机 nonce 加密原文。恢复密钥从既有 PAT 密钥通过独立 HKDF context 派生，AAD 绑定 identity ID 与 Token hash，防止交换密文。持久化部署主密钥是恢复条件，密钥备份和轮换必须同时考虑现有 HMAC 验证及恢复密文，不能直接换密钥后期待旧 Token 可用。

旧记录只有摘要，无法恢复原文；返回明确 recovery-unavailable 409。不自动轮换、删除或重建旧 Token。上线须先执行 0035 迁移。
