import {
  addImports,
  addServerImports,
  createResolver,
  defineNuxtModule,
} from "@nuxt/kit";

export default defineNuxtModule({
  meta: {
    name: "@yueli/discovery-nuxt",
    configKey: "discovery",
  },
  setup() {
    const resolver = createResolver(import.meta.url);
    addImports({
      name: "useDiscoveryPage",
      from: resolver.resolve("./runtime/composable"),
    });
    addServerImports({
      name: "serveDiscoveryArtifact",
      from: resolver.resolve("./runtime/server/discovery"),
    });
  },
});
