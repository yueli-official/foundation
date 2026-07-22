# Local consumption

Foundation can be consumed directly from a sibling checkout before npm and Go releases exist. This is a development-only seam: consumers import the public package/module names, while workspace configuration redirects those names to local source.

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

Local consumption is supported and used by Platform, but it is not a substitute for a published version. CI or deployment environments that do not check out the sibling repository must wait for npm prereleases and the `go/v*` module tag.
