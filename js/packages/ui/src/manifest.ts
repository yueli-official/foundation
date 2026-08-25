export type PublicUiMaturity =
  "experimental" | "alpha" | "beta" | "stable" | "deprecated";
export type PublicUiKind =
  "contract" | "headless-workflow" | "pattern" | "adapter";

export interface PublicUiManifestEntry {
  readonly id: string;
  readonly kind: PublicUiKind;
  readonly status: PublicUiMaturity;
  readonly entrypoint: `@yueli/ui/${string}`;
  readonly owner: "foundation";
  readonly responsibility: string;
  readonly nonResponsibilities: readonly string[];
  readonly runtimeDependencies: readonly string[];
  readonly replacement?: `@yueli/ui/${string}`;
  readonly removalVersion?: string;
  readonly evidence: {
    readonly docs: readonly string[];
    readonly tests: readonly string[];
    readonly consumers: readonly string[];
    readonly accessibility: readonly string[];
  };
}

export const publicUiManifest = [
  {
    id: "theme",
    kind: "contract",
    status: "experimental",
    entrypoint: "@yueli/ui/theme",
    owner: "foundation",
    responsibility:
      "Own a provider-neutral Nuxt UI preset, deterministic local Tabler delivery and opt-in semantic light/dark surface tokens.",
    nonResponsibilities: [
      "application theme names",
      "product color registration",
      "runtime theme persistence",
      "locale catalogs",
    ],
    runtimeDependencies: [
      "@iconify-json/tabler",
      "@nuxt/icon",
      "@nuxt/ui",
      "tailwindcss",
    ],
    evidence: {
      docs: ["README.md"],
      tests: [
        "test/theme.test.ts",
        "test/icon-delivery.test.ts",
        "scripts/test-pack.mjs",
      ],
      consumers: ["js/conformance/ui"],
      accessibility: ["test/theme.test.ts"],
    },
  },
  {
    id: "image-policy",
    kind: "contract",
    status: "experimental",
    entrypoint: "@yueli/ui/image",
    owner: "foundation",
    responsibility:
      "Own bounded raster optimization decisions, resize geometry, cropper configuration, output naming and caller-translatable dimension violations.",
    nonResponsibilities: [
      "asset upload HTTP",
      "media storage",
      "visible cropper copy",
      "image CDN transformations",
    ],
    runtimeDependencies: [],
    evidence: {
      docs: ["README.md"],
      tests: ["test/image.test.ts", "scripts/test-pack.mjs"],
      consumers: ["js/conformance/ui"],
      accessibility: [],
    },
  },
  {
    id: "browser-image-optimization",
    kind: "adapter",
    status: "experimental",
    entrypoint: "@yueli/ui/image/browser",
    owner: "foundation",
    responsibility:
      "Apply the public image policy through a disposable browser decode/canvas/file runtime and report explicit fallback reasons.",
    nonResponsibilities: [
      "upload retries",
      "progress UI",
      "server-side transcoding",
      "cropper rendering",
    ],
    runtimeDependencies: [],
    evidence: {
      docs: ["README.md"],
      tests: ["test/image.test.ts", "scripts/test-pack.mjs"],
      consumers: ["js/conformance/ui"],
      accessibility: [],
    },
  },
  {
    id: "messages",
    kind: "contract",
    status: "experimental",
    entrypoint: "@yueli/ui/messages",
    owner: "foundation",
    responsibility:
      "Carry stable caller-resolved message keys and JSON-safe parameters.",
    nonResponsibilities: [
      "locale catalogs",
      "backend raw messages",
      "Nuxt i18n configuration",
    ],
    runtimeDependencies: [],
    evidence: {
      docs: ["README.md"],
      tests: [],
      consumers: ["js/conformance/ui"],
      accessibility: [],
    },
  },
  {
    id: "collection-workflow",
    kind: "headless-workflow",
    status: "experimental",
    entrypoint: "@yueli/ui/collection",
    owner: "foundation",
    responsibility:
      "Own remote collection load sequencing, route query normalization, query scope and cross-page selection invariants.",
    nonResponsibilities: [
      "business HTTP",
      "Vue rendering",
      "Nuxt UI primitives",
      "domain bulk actions",
    ],
    runtimeDependencies: [],
    evidence: {
      docs: ["README.md"],
      tests: [
        "test/collection-workflow.test.ts",
        "test/query-sync.test.ts",
        "js/conformance/ui/test/e2e/collection.spec.ts",
        "scripts/test-pack.mjs",
      ],
      consumers: [
        "js/conformance/ui",
        "yueli-official/platform/products/resource/web",
        "yueli-official/platform/products/blog/web",
      ],
      accessibility: ["js/conformance/ui/test/e2e/collection.spec.ts"],
    },
  },
  {
    id: "action-feedback",
    kind: "headless-workflow",
    status: "experimental",
    entrypoint: "@yueli/ui/feedback",
    owner: "foundation",
    responsibility:
      "Own latest-wins async action state, terminal reset timing, stable loading visibility and transport-neutral notice normalization.",
    nonResponsibilities: [
      "business mutations",
      "locale catalogs",
      "global toast installation",
    ],
    runtimeDependencies: ["vue"],
    evidence: {
      docs: ["README.md"],
      tests: [
        "test/action-feedback.test.ts",
        "test/minimum-loading.test.ts",
        "test/feedback-notice.test.ts",
      ],
      consumers: ["js/conformance/ui"],
      accessibility: [],
    },
  },
  {
    id: "action-feedback-button",
    kind: "pattern",
    status: "experimental",
    entrypoint: "@yueli/ui/feedback/pattern",
    owner: "foundation",
    responsibility:
      "Render a Nuxt UI action control whose label, icon, tone and live state remain coherent across idle, pending, success and error.",
    nonResponsibilities: [
      "executing mutations",
      "locale catalogs",
      "toast feedback",
    ],
    runtimeDependencies: ["nuxt", "@nuxt/ui", "tailwindcss", "vue"],
    evidence: {
      docs: ["README.md"],
      tests: ["test/action-feedback-button.test.ts"],
      consumers: ["js/conformance/ui"],
      accessibility: ["test/action-feedback-button.test.ts"],
    },
  },
  {
    id: "back-to-top",
    kind: "pattern",
    status: "experimental",
    entrypoint: "@yueli/ui/navigation/back-to-top",
    owner: "foundation",
    responsibility:
      "Own scroll threshold, window or container scrolling, focus return, reduced motion and overlay/dock avoidance for one accessible floating control.",
    nonResponsibilities: [
      "page layout",
      "locale catalogs",
      "overlay state management",
    ],
    runtimeDependencies: ["nuxt", "@nuxt/ui", "tailwindcss", "vue"],
    evidence: {
      docs: ["README.md"],
      tests: ["test/back-to-top.test.ts"],
      consumers: ["js/conformance/ui"],
      accessibility: ["test/back-to-top.test.ts"],
    },
  },
  {
    id: "reading-table-of-contents",
    kind: "pattern",
    status: "experimental",
    entrypoint: "@yueli/ui/navigation/table-of-contents",
    owner: "foundation",
    responsibility:
      "Own heading filtering, hierarchy, fragment navigation, active-section tracking and accessible visual states for in-page reading navigation.",
    nonResponsibilities: [
      "article rendering",
      "page column layout",
      "mobile disclosure placement",
      "product color values",
    ],
    runtimeDependencies: ["@nuxt/ui", "tailwindcss", "vue"],
    evidence: {
      docs: ["README.md"],
      tests: ["test/reading-table-of-contents.test.ts"],
      consumers: ["docs/web", "blog/web"],
      accessibility: ["test/reading-table-of-contents.test.ts"],
    },
  },
  {
    id: "account-menu-pattern",
    kind: "pattern",
    status: "experimental",
    entrypoint: "@yueli/ui/account-menu/pattern",
    owner: "foundation",
    responsibility:
      "Own provider-neutral identity fallback, account and appearance action grouping, inline/sidebar/collapsed trigger anatomy and the async logout command boundary.",
    nonResponsibilities: [
      "identity providers",
      "session state",
      "application navigation",
      "locale catalogs",
      "color-mode state or persistence",
    ],
    runtimeDependencies: ["nuxt", "@nuxt/ui", "tailwindcss", "vue"],
    evidence: {
      docs: ["README.md"],
      tests: ["test/account-menu.test.ts", "scripts/test-pack.mjs"],
      consumers: ["js/conformance/ui"],
      accessibility: ["test/account-menu.test.ts"],
    },
  },
  {
    id: "admin-template",
    kind: "pattern",
    status: "experimental",
    entrypoint: "@yueli/ui/admin",
    owner: "foundation",
    responsibility:
      "Own the reusable admin-console shell, sidebar search/navigation, route bar, semantic canvas, page headers, tabbed surfaces and collection-toolbar anatomy with caller-owned brand content, routes and domain data.",
    nonResponsibilities: [
      "business dashboard regions",
      "application routes",
      "authorization",
      "brand assets",
      "locale catalogs",
    ],
    runtimeDependencies: ["nuxt", "@nuxt/ui", "tailwindcss", "vue"],
    evidence: {
      docs: ["README.md"],
      tests: [
        "test/admin-navigation.test.ts",
        "test/admin-template.test.ts",
        "scripts/test-pack.mjs",
      ],
      consumers: [
        "js/apps/ui-lab",
        "js/conformance/ui",
        "yueli-official/blog/web",
        "yueli-official/identity/account",
      ],
      accessibility: ["test/admin-template.test.ts"],
    },
  },
  {
    id: "remote-select",
    kind: "pattern",
    status: "experimental",
    entrypoint: "@yueli/ui/remote-select",
    owner: "foundation",
    responsibility:
      "Own debounced, abortable, latest-wins remote option loading, per-instance query caching and retry around Nuxt UI SelectMenu.",
    nonResponsibilities: [
      "business HTTP clients",
      "local option filtering",
      "multi-select",
      "creating domain entities",
      "locale catalogs",
    ],
    runtimeDependencies: ["nuxt", "@nuxt/ui", "tailwindcss", "vue"],
    evidence: {
      docs: ["README.md"],
      tests: ["test/remote-select.test.ts", "scripts/test-pack.mjs"],
      consumers: ["js/apps/ui-lab", "js/conformance/ui"],
      accessibility: ["test/remote-select.test.ts"],
    },
  },
  {
    id: "dashboard-patterns",
    kind: "pattern",
    status: "deprecated",
    entrypoint: "@yueli/ui/dashboard/pattern",
    owner: "foundation",
    responsibility:
      "Own responsive page-heading anatomy and the accessible decision order for caller-provided dashboard regions.",
    nonResponsibilities: [
      "business metrics",
      "business actions",
      "locale catalogs",
      "application navigation",
    ],
    runtimeDependencies: ["nuxt", "@nuxt/ui", "tailwindcss", "vue"],
    replacement: "@yueli/ui/admin",
    removalVersion: "0.2.0",
    evidence: {
      docs: ["README.md"],
      tests: ["test/dashboard-patterns.test.ts", "scripts/test-pack.mjs"],
      consumers: ["js/conformance/ui"],
      accessibility: ["test/dashboard-patterns.test.ts"],
    },
  },
  {
    id: "settings-workflow",
    kind: "headless-workflow",
    status: "experimental",
    entrypoint: "@yueli/ui/settings",
    owner: "foundation",
    responsibility:
      "Own JSON-safe settings baseline cloning, structural dirty comparison, capture, discard and revision semantics.",
    nonResponsibilities: [
      "business persistence",
      "Vue rendering",
      "browser navigation",
      "locale catalogs",
    ],
    runtimeDependencies: [],
    evidence: {
      docs: ["README.md"],
      tests: ["test/settings-workflow.test.ts", "scripts/test-pack.mjs"],
      consumers: ["js/conformance/ui"],
      accessibility: [],
    },
  },
  {
    id: "settings-adapters",
    kind: "adapter",
    status: "experimental",
    entrypoint: "@yueli/ui/settings/vue",
    owner: "foundation",
    responsibility:
      "Bind a caller-owned settings form to Vue reactivity and opt-in browser or Router leave protection.",
    nonResponsibilities: [
      "confirmation copy",
      "persistence",
      "route query semantics",
      "global window policy",
    ],
    runtimeDependencies: ["vue", "vue-router"],
    evidence: {
      docs: ["README.md"],
      tests: [
        "test/settings-vue.test.ts",
        "test/settings-browser.test.ts",
        "test/settings-vue-router.test.ts",
      ],
      consumers: ["js/conformance/ui"],
      accessibility: [],
    },
  },
  {
    id: "settings-patterns",
    kind: "pattern",
    status: "experimental",
    entrypoint: "@yueli/ui/settings/pattern",
    owner: "foundation",
    responsibility:
      "Own responsive settings navigation, section anatomy and a safe-area-aware save lifecycle dock.",
    nonResponsibilities: [
      "settings fields",
      "business persistence",
      "permissions",
      "locale catalogs",
    ],
    runtimeDependencies: ["nuxt", "@nuxt/ui", "tailwindcss", "vue"],
    evidence: {
      docs: ["README.md"],
      tests: ["test/settings-patterns.test.ts", "scripts/test-pack.mjs"],
      consumers: ["js/conformance/ui"],
      accessibility: ["test/settings-patterns.test.ts"],
    },
  },
  {
    id: "collection-frame",
    kind: "pattern",
    status: "experimental",
    entrypoint: "@yueli/ui/collection/pattern",
    owner: "foundation",
    responsibility:
      "Own the integrated collection frame anatomy, responsive controls disclosure, in-flow bulk region and accessible structural defaults.",
    nonResponsibilities: [
      "filter and sort semantics",
      "domain columns and items",
      "business actions",
      "pagination primitives",
      "translated copy",
      "data-table column sizing and alignment owned by Nuxt UI Table",
    ],
    runtimeDependencies: ["nuxt", "@nuxt/ui", "tailwindcss", "vue"],
    evidence: {
      docs: ["README.md"],
      tests: [
        "js/conformance/ui/test/e2e/collection.spec.ts",
        "scripts/test-pack.mjs",
      ],
      consumers: ["js/conformance/ui"],
      accessibility: ["js/conformance/ui/test/e2e/collection.spec.ts"],
    },
  },
  {
    id: "collection-table-toolbar",
    kind: "pattern",
    status: "experimental",
    entrypoint: "@yueli/ui/collection/pattern",
    owner: "foundation",
    responsibility:
      "Own one responsive default toolbar, stable filter popover and in-place selection mode around a caller-owned Nuxt UI Table.",
    nonResponsibilities: [
      "data-table rendering and column alignment",
      "filter and sort semantics",
      "business bulk actions",
      "business HTTP",
      "locale catalogs",
    ],
    runtimeDependencies: ["nuxt", "@nuxt/ui", "tailwindcss", "vue"],
    evidence: {
      docs: ["README.md", "src/collection/toolbar-standard.md"],
      tests: ["test/collection-table-toolbar.test.ts", "scripts/test-pack.mjs"],
      consumers: ["yueli-official/platform/products/docs/web"],
      accessibility: ["test/collection-table-toolbar.test.ts"],
    },
  },
  {
    id: "collection-panel",
    kind: "pattern",
    status: "experimental",
    entrypoint: "@yueli/ui/collection/pattern",
    owner: "foundation",
    responsibility:
      "Own complete responsive search, configured controls, in-flow selection and bulk actions, loading/error/empty states, row/grid containers and pagination for remote collections.",
    nonResponsibilities: [
      "business HTTP",
      "domain item fields",
      "strict data-table column alignment",
      "business bulk actions",
      "locale catalogs",
    ],
    runtimeDependencies: ["nuxt", "@nuxt/ui", "tailwindcss", "vue"],
    evidence: {
      docs: ["README.md"],
      tests: [
        "test/collection-panel.test.ts",
        "js/conformance/ui/test/e2e/collection.spec.ts",
        "scripts/test-pack.mjs",
      ],
      consumers: ["js/conformance/ui", "js/apps/ui-lab"],
      accessibility: ["js/conformance/ui/test/e2e/collection.spec.ts"],
    },
  },
  {
    id: "collection-vue-adapter",
    kind: "adapter",
    status: "experimental",
    entrypoint: "@yueli/ui/collection/vue",
    owner: "foundation",
    responsibility:
      "Bind a client-mounted collection Workflow to Vue setup lifecycle, reactive snapshots and data-query invalidation.",
    nonResponsibilities: [
      "SSR data fetching",
      "business HTTP",
      "search input policy",
      "Vue rendering",
      "domain feedback",
    ],
    runtimeDependencies: ["vue"],
    evidence: {
      docs: ["README.md"],
      tests: ["test/vue-collection-workflow.test.ts", "scripts/test-pack.mjs"],
      consumers: [
        "js/conformance/ui",
        "yueli-official/platform/products/resource/web",
        "yueli-official/platform/products/blog/web",
      ],
      accessibility: [],
    },
  },
  {
    id: "collection-vue-router-adapter",
    kind: "adapter",
    status: "experimental",
    entrypoint: "@yueli/ui/collection/vue-router",
    owner: "foundation",
    responsibility:
      "Synchronize an owned collection query subset with Vue Router while preserving unrelated query keys.",
    nonResponsibilities: [
      "query semantics",
      "business HTTP",
      "route registration",
    ],
    runtimeDependencies: ["vue", "vue-router"],
    evidence: {
      docs: ["README.md"],
      tests: [
        "test/vue-router-query-sync.test.ts",
        "js/conformance/ui/test/e2e/collection.spec.ts",
        "scripts/test-pack.mjs",
      ],
      consumers: [
        "js/conformance/ui",
        "yueli-official/platform/products/resource/web",
        "yueli-official/platform/products/blog/web",
      ],
      accessibility: [],
    },
  },
] as const satisfies readonly PublicUiManifestEntry[];
