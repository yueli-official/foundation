import {
  addComponentsDir,
  addImports,
  createResolver,
  defineNuxtModule,
} from "@nuxt/kit";
import { createTablerIconDelivery } from "./icon-delivery";

export interface YueliUiModuleOptions {
  /** Prefix for auto-imported public components. */
  prefix?: string;
  /**
   * Finite Tabler icon names supplied by persisted data or APIs. Literal icon
   * names in source files are discovered automatically.
   */
  tablerIcons?: string[];
}

const yueliUiModule = defineNuxtModule<YueliUiModuleOptions>({
  meta: {
    name: "@yueli/ui",
    configKey: "yueliUi",
  },
  defaults: {
    prefix: "Y",
    tablerIcons: [],
  },
  moduleDependencies(nuxt): {
    "@nuxt/icon": {
      defaults: ReturnType<typeof createTablerIconDelivery>;
    };
  } {
    const configured: string[] | undefined = (
      nuxt.options as unknown as { yueliUi?: YueliUiModuleOptions }
    ).yueliUi?.tablerIcons;

    return {
      "@nuxt/icon": {
        defaults: createTablerIconDelivery(configured),
      },
    };
  },
  setup(options, nuxt) {
    const resolver = createResolver(import.meta.url);
    const packageManifest = resolver.resolve("../package.json");
    if (!nuxt.options.watch.includes(packageManifest)) {
      // Vite caches package export maps. Local-source consumers must restart
      // their Nuxt resolver when Foundation adds a new public subpath.
      nuxt.options.watch.push(packageManifest);
    }

    addImports([
      {
        name: "useActionFeedback",
        from: resolver.resolve("./feedback/index"),
      },
      {
        name: "useMinimumLoading",
        from: resolver.resolve("./feedback/index"),
      },
    ]);

    for (const directory of [
      "account-menu",
      "admin",
      "collection",
      "comments",
      "dashboard",
      "feedback",
      "navigation",
      "remote-select",
      "sharing",
      "settings",
    ]) {
      addComponentsDir({
        path: resolver.resolve(`./${directory}/components`),
        pathPrefix: false,
        prefix: options.prefix,
      });
    }
  },
});

export default yueliUiModule;
