# GoFrame API middleware

`goframe/api` owns one request lifecycle: trace ID propagation, optional
caller-owned rate limiting, raw success DTO serialization, immutable
`problem.Error` mapping, GoFrame validation violations and generic internal
failure handling.

Applications supply their public Problem descriptors, limiter and client-key
topology. The module does not read environment variables, choose trusted
proxies or register application error codes.

验证：

```text
go test ./goframe/api
```
