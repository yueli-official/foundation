# Yueli Access Token Profile v1

本合同用于 Yueli OAuth resource server，不用于 ID token、step-up proof 或任意通用 JWT。

## JOSE header

- `typ` 必须为 `at+jwt` 或 `application/at+jwt`。
- `alg` 必须属于 resource server 配置的非对称算法 allowlist。
- `kid` 必填，并只能在配置期绑定的 issuer JWKS 中解析。
- 对应 JWK 必须是 public signing key，声明唯一 `kid`、`use: sig` 和 `alg`；JWK `alg` 必须与 header `alg` 一致。
- 不读取 token 提供的 `jku`、`x5u` 或其他远程 key 地址。

## Claims

| Claim          | 规则                                                     |
| -------------- | -------------------------------------------------------- |
| `iss`          | 必填，与配置的受信 issuer 精确匹配                       |
| `aud`          | 必填，至少命中 resource server 配置的一个 audience       |
| `exp`          | 必填，结合有限 leeway 验证                               |
| `iat`          | 建议必填；配置最大 lifetime 时强制                       |
| `sub`          | `user`/`guest` 必填；`client` 可携带但不作为 actor key   |
| `client_id`    | `client` 必填；delegated user 可附带调用 client          |
| `subject_kind` | 必填，枚举 `user`、`client`、`guest`                     |
| `scope`        | 可选，空格分隔、去重后的授权 scope                       |
| `roles`        | 可选；只表达 producer 明确签发的角色，不替代资源服务授权 |

Actor 选择没有兼容猜测：`user`/`guest` 使用 `sub`，`client` 使用 `client_id`。未知或缺失 `subject_kind` 一律拒绝。

## 版本迁移

生产者先具备签发 v1 header/claims 的能力，消费者在同一发布窗口切换到严格 verifier；conformance 同时覆盖 user、client、guest、错误 audience、错误 type、缺失/不匹配 JWK algorithm。完成后删除接受通用 `JWT` 或猜测 actor 的临时代码。
