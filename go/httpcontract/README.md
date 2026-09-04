# HTTP Contract

`httpcontract` compiles and verifies product-owned public error catalogs and HTTP operation result declarations. It standardizes failure semantics and success status/body shapes without wrapping successful DTOs in `{code,data,message}`.

## Interface

- `ParseErrorCatalog` and `ParseOperations` strictly decode and validate declarations.
- `VerifyReferences` rejects operation references to undeclared product errors while preserving Foundation `common.*` and protocol-native OAuth errors.
- `DiffErrorCatalogs` and `DiffOperations` classify breaking, behavioral and additive changes.
- `GenerateGo`, `GenerateTypeScript` and `GenerateI18nInventory` produce deterministic consumer artifacts.
- `cmd/httpcontract` exposes validation, generation, freshness checking and compatibility diff to CI.

Product DTO schemas remain owned by OpenAPI. Operation declarations reference them and only own the success status/body kind. OAuth/OIDC endpoints declare `failureProtocol: oauth`; they are not projected as RFC 9457 Problem responses.

```powershell
go run ./httpcontract/cmd/httpcontract `
  -errors ./contracts/http-result/error-catalog.json `
  -operations ./contracts/http-result/operations.json `
  -generate-go ./internal/producterr/catalog_gen.go -package producterr `
  -generate-ts ./web/generated/productFailure.ts -ts-type ProductFailure `
  -generate-i18n ./web/generated/error-i18n-inventory.json `
  -check
```
