# Local consumption

Foundation 可以在开发期从相邻 checkout 消费。正式 Go 与 JS 制品已经存在，因此这只是一条本地联调 seam：
消费者仍导入公共 package/module 名称，由 workspace 配置把它们临时重定向到本地源码。

Assume this layout:

```text
yueli-official/
  foundation/
  your-app/
```

## pnpm / Nuxt

Add the Foundation packages to the consumer repository's `pnpm-workspace.yaml`:

```yaml
packages:
  - "apps/*"
  - "packages/*"
  - "../foundation/js/packages/*"
```

Declare only the packages the app uses:

```json
{
  "dependencies": {
    "@yueli/http-runtime": "workspace:*",
    "@yueli/nuxt-runtime": "workspace:*",
    "@yueli/ui": "workspace:*"
  }
}
```

Then install from the consumer root:

```sh
pnpm install
pnpm why @yueli/ui
```

The resolved path should point into `foundation/js/packages`. Do not copy package source or manually edit `node_modules`. Nuxt apps still add the required modules and CSS through each package's public README.

## Go / GoFrame

Keep the public module path in the consumer `go.mod`:

```go
require github.com/yueli-official/foundation/go v0.0.0
```

Redirect it from the consumer repository's `go.work`:

```go
go 1.25.12

use ./path/to/your/module

replace github.com/yueli-official/foundation/go => ../foundation/go
```

Adjust the relative path for the repository layout, then verify resolution:

```sh
go list -m -json github.com/yueli-official/foundation/go
go test ./...
```

`Dir` should point to the sibling Foundation checkout. Keep the `replace` in `go.work`, not every application `go.mod`, so release dependencies remain clean.

## Current limitation

本地消费不能替代正式制品。CI、部署和发布验收必须移除本地 workspace/`replace`，分别安装已发布的 GitHub
Release tarball 和 `go/v*` module tag；具体命令见 [消费者接入指南](consumer-integration.md)。
