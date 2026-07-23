# Discovery

`discovery` is the Foundation deep module for site discovery protocols. Products
provide public page facts and stable cursor sources; the module derives one
canonical identity and projects it consistently into page metadata, JSON-LD,
sitemap/index, RSS/Atom and robots artifacts.

```go
module := discovery.MustCompile(discovery.Definition{
    ContractVersion: discovery.ContractVersion,
    Site: discovery.SiteProfile{
        Origin: "https://docs.example.com",
        Name: "Docs",
        DefaultLocale: "zh-CN",
    },
})

projection, report, err := module.Project(discovery.PageDescriptor{
    Key: "doc:install",
    Path: "/guide/install",
    Subject: discovery.WebPageSubject(discovery.WebPage{
        Title: "Install",
        Description: "Installation guide",
    }),
})
```

`Publish` reads stable keyset-ordered `CursorSource` records and writes through
a staged `PublishTarget`. A target exposes nothing until every artifact has
closed and the manifest commits. Source, encoding and target failures therefore
cannot silently replace a healthy sitemap with an empty response.

Important constraints:

- Configure the canonical origin; never derive it from an HTTP request.
- Private/auth-only records must not enter Discovery.
- Unlisted records become `noindex,follow` and cannot enter a sitemap.
- Sitemap sources use canonical URL as their stable `SortKey`.
- All content and publication times are explicit; generation has no hidden
  wall-clock dependency.
- Feed HTML is sanitized by a fixed internal policy.

Use `MemorySource` and `MemoryTarget` for tests. Production products should keep
their database query and artifact serving code in local Adapters.
