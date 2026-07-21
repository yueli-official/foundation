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
