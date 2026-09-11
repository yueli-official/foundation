# Auth

`auth` 提供与 HTTP 框架无关的签名 JWT 验证。它不发现 issuer、不从 token 读取 JWKS URL，也不拥有授权决策。

## 两个入口

- `NewAccessTokenVerifier`：OAuth resource server 的安全默认入口。要求 audience，默认只接受 RFC 9068
  `at+jwt` 类型，并要求 JWK `alg` 与 token `alg` 绑定。
- `NewVerifier`：step-up proof 等显式自定义 JWT 类型。调用者必须配置 audience/type 等互斥规则；不要把空配置的通用 verifier 当作 access-token verifier。

`jwks.StaticSource` 和 `jwks.RemoteSource` 会保留 JWK metadata，因此可直接用于严格 access-token verifier。只返回裸 public key 的自定义 `KeySource` 不能通过严格入口。

Yueli 自定义 actor claim 见 [Access Token Profile](../../contracts/auth/access-token-profile.md)。

## 个人令牌

`NewPersonalTokenVerifier` 在线验证 PAT，`CompositeVerifier` 按保留前缀组合 JWT/PAT。
`PersonalScope` 将具体站点实例与单个能力绑定；`AllowsPersonalCapability` 只增加限制，
不能替代消费者当前业务授权。`PersonalCatalog` 查询可信消费者当前用户的可委托能力。
无有效令牌正缓存，失败不回退到浏览器身份。接入配置和验收状态见
[个人令牌合同](../../flightdeck/work/2026-09-09-personal-token-authorization/integration.md)。
