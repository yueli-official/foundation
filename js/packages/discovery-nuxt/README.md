# `@yueli/discovery-nuxt`

Nuxt framework Adapter for the versioned projection produced by Foundation
Discovery.

```ts
export default defineNuxtConfig({
  modules: ["@yueli/discovery-nuxt"],
});
```

```ts
const { data: article } = await useFetch("/api/articles/example");
useDiscoveryPage(() => article.value?.discovery);
```

The Adapter validates the projection, installs deterministic head keys and
serializes JSON-LD safely. It does not derive canonical URLs, decide index
policy, enumerate product records or render a second sitemap.

For non-HTML responses, use `applyDiscoveryHeaders` from
`@yueli/discovery-nuxt/server`.
