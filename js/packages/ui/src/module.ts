import { addComponentsDir, createResolver, defineNuxtModule } from "@nuxt/kit";

export interface YueliUiModuleOptions {
  /** Prefix for auto-imported public components. */
  prefix?: string;
}

export default defineNuxtModule<YueliUiModuleOptions>({
  meta: {
    name: "@yueli/ui",
    configKey: "yueliUi",
  },
  defaults: {
    prefix: "Y",
  },
  setup(options) {
    const resolver = createResolver(import.meta.url);

    addComponentsDir({
      path: resolver.resolve("./collection/components"),
      pathPrefix: false,
      prefix: options.prefix,
    });
  },
});
