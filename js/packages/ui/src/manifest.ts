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
  readonly evidence: {
    readonly docs: readonly string[];
    readonly tests: readonly string[];
    readonly consumers: readonly string[];
    readonly accessibility: readonly string[];
  };
}

export const publicUiManifest = [
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
    id: "account-menu-pattern",
    kind: "pattern",
    status: "experimental",
    entrypoint: "@yueli/ui/account-menu/pattern",
    owner: "foundation",
    responsibility:
      "Own provider-neutral identity fallback, account action grouping, accessible trigger anatomy and the async logout command boundary.",
    nonResponsibilities: [
      "identity providers",
      "session state",
      "application navigation",
      "locale catalogs",
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
    id: "dashboard-patterns",
    kind: "pattern",
    status: "experimental",
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
      "Own the integrated collection frame anatomy, responsive controls disclosure, sticky bulk region and accessible structural defaults.",
    nonResponsibilities: [
      "filter and sort semantics",
      "domain columns and items",
      "business actions",
      "pagination primitives",
      "translated copy",
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
    id: "collection-panel",
    kind: "pattern",
    status: "experimental",
    entrypoint: "@yueli/ui/collection/pattern",
    owner: "foundation",
    responsibility:
      "Own the complete responsive search, configured controls, selection, bulk, loading/error/empty, row/grid and pagination anatomy for remote collections.",
    nonResponsibilities: [
      "business HTTP",
      "domain item fields",
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
