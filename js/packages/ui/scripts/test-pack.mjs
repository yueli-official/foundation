import { spawn } from "node:child_process";
import {
  mkdtemp,
  mkdir,
  readFile,
  readdir,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const temporaryRoot = await mkdtemp(join(tmpdir(), "yueli-ui-pack-"));
const consumerRoot = join(temporaryRoot, "consumer");
const tar = process.platform === "win32" ? "tar.exe" : "tar";
const corepack = join(
  dirname(process.execPath),
  "node_modules",
  "corepack",
  "dist",
  "corepack.js",
);
const pnpm = process.platform === "win32" ? process.execPath : "pnpm";
const pnpmPrefix = process.platform === "win32" ? [corepack, "pnpm"] : [];
const resolvedTemporaryRoot = resolve(temporaryRoot);

if (
  dirname(resolvedTemporaryRoot) !== resolve(tmpdir()) ||
  !basename(resolvedTemporaryRoot).startsWith("yueli-ui-pack-")
) {
  throw new Error(
    `Refusing to use unexpected temporary directory: ${resolvedTemporaryRoot}`,
  );
}

function run(command, args, options = {}) {
  return new Promise((resolveRun, rejectRun) => {
    const child = spawn(command, args, {
      cwd: options.cwd ?? packageRoot,
      env: { ...process.env, CI: "true", ...options.env },
      stdio: options.capture ? ["ignore", "pipe", "inherit"] : "inherit",
      shell: false,
    });
    let output = "";
    if (options.capture)
      child.stdout.on("data", (chunk) => {
        output += chunk;
      });
    child.on("error", rejectRun);
    child.on("exit", (code) => {
      if (code === 0) resolveRun(output);
      else
        rejectRun(
          new Error(`${command} ${args.join(" ")} exited with ${code}.`),
        );
    });
  });
}

function runPnpm(args, options) {
  return run(pnpm, [...pnpmPrefix, ...args], options);
}

function toFileSpecifier(path) {
  return `file:${path.replaceAll("\\", "/")}`;
}

try {
  await runPnpm(["pack", "--pack-destination", temporaryRoot]);
  const tarballName = (await readdir(temporaryRoot)).find((entry) =>
    entry.endsWith(".tgz"),
  );
  if (!tarballName) throw new Error("pnpm pack did not create a tarball.");
  const tarball = join(temporaryRoot, tarballName);
  const packedFiles = (await run(tar, ["-tf", tarball], { capture: true }))
    .split(/\r?\n/u)
    .filter(Boolean)
    .sort();
  const allowedFiles = [
    "package/LICENSE",
    "package/README.md",
    "package/package.json",
    "package/src/account-menu/components/AccountMenu.vue",
    "package/src/account-menu/pattern.ts",
    "package/src/admin/components/AdminConsoleLayout.vue",
    "package/src/admin/components/AdminIconPicker.vue",
    "package/src/admin/components/AdminPage.vue",
    "package/src/admin/components/AdminRowActions.vue",
    "package/src/admin/components/AdminShell.vue",
    "package/src/admin/components/EditorInspector.vue",
    "package/src/admin/components/ManagePage.vue",
    "package/src/admin/icons.ts",
    "package/src/admin/index.ts",
    "package/src/admin/navigation.ts",
    "package/src/admin/types.ts",
    "package/src/collection/components/CollectionActiveFilters.vue",
    "package/src/collection/components/CollectionDock.vue",
    "package/src/collection/components/CollectionFooter.vue",
    "package/src/collection/components/CollectionFrame.vue",
    "package/src/collection/components/CollectionLifecycleTabs.vue",
    "package/src/collection/components/CollectionPageSelection.vue",
    "package/src/collection/components/CollectionPagination.vue",
    "package/src/collection/components/CollectionPanel.vue",
    "package/src/collection/components/CollectionRowShell.vue",
    "package/src/collection/components/CollectionSortDirectionButton.vue",
    "package/src/collection/components/CollectionSortHeader.vue",
    "package/src/collection/components/CollectionTableToolbar.vue",
    "package/src/collection/components/CollectionToolbar.vue",
    "package/src/collection/components/CollectionViewToggle.vue",
    "package/src/collection/index.ts",
    "package/src/collection/panel.ts",
    "package/src/collection/pattern.ts",
    "package/src/collection/route-query.ts",
    "package/src/collection/toolbar-standard.md",
    "package/src/collection/vue.ts",
    "package/src/collection/vue-router.ts",
    "package/src/collection/workflow.ts",
    "package/src/comments/admin/components/CommentModerationCollection.vue",
    "package/src/comments/admin/index.ts",
    "package/src/comments/admin/types.ts",
    "package/src/comments/components/PublicCommentComposer.vue",
    "package/src/comments/components/PublicCommentThread.vue",
    "package/src/comments/icons.ts",
    "package/src/comments/index.ts",
    "package/src/comments/types.ts",
    "package/src/dashboard/components/DashboardLayout.vue",
    "package/src/dashboard/components/PageHeader.vue",
    "package/src/dashboard/components/TabbedSurface.vue",
    "package/src/dashboard/pattern.ts",
    "package/src/feedback/action.ts",
    "package/src/feedback/components/ActionFeedbackButton.vue",
    "package/src/feedback/components/FeedbackToastRegion.client.vue",
    "package/src/feedback/index.ts",
    "package/src/feedback/minimum-loading.ts",
    "package/src/feedback/notice.ts",
    "package/src/feedback/pattern.ts",
    "package/src/icon-delivery.ts",
    "package/src/image/browser.ts",
    "package/src/image/index.ts",
    "package/src/manifest.ts",
    "package/src/messages.ts",
    "package/src/module.ts",
    "package/src/navigation/back-to-top.ts",
    "package/src/navigation/components/BackToTop.vue",
    "package/src/navigation/components/ReadingTableOfContents.vue",
    "package/src/navigation/table-of-contents.ts",
    "package/src/navigation/table-of-contents.types.ts",
    "package/src/sharing/components/ContentShareActions.vue",
    "package/src/sharing/content-share.ts",
    "package/src/sharing/content-share.types.ts",
    "package/src/remote-select/components/RemoteSelect.vue",
    "package/src/remote-select/index.ts",
    "package/src/remote-select/types.ts",
    "package/src/settings/browser.ts",
    "package/src/settings/components/SettingSection.vue",
    "package/src/settings/components/SettingsLayout.vue",
    "package/src/settings/components/SettingsSaveDock.vue",
    "package/src/settings/index.ts",
    "package/src/settings/pattern.ts",
    "package/src/settings/vue-router.ts",
    "package/src/settings/vue.ts",
    "package/src/settings/workflow.ts",
    "package/src/tailwind.css",
    "package/src/theme.css",
    "package/src/theme/index.d.ts",
    "package/src/theme/index.js",
  ].sort();
  if (JSON.stringify(packedFiles) !== JSON.stringify(allowedFiles)) {
    const unexpected = packedFiles.filter(
      (file) => !allowedFiles.includes(file),
    );
    const missing = allowedFiles.filter((file) => !packedFiles.includes(file));
    throw new Error(
      [
        "Unexpected tarball contents",
        `Added:\n${unexpected.join("\n") || "(none)"}`,
        `Missing:\n${missing.join("\n") || "(none)"}`,
      ].join("\n"),
    );
  }

  await mkdir(join(consumerRoot, "app", "assets", "css"), {
    recursive: true,
  });
  await writeFile(
    join(consumerRoot, "package.json"),
    `${JSON.stringify(
      {
        name: "yueli-ui-packed-consumer",
        private: true,
        type: "module",
        packageManager: "pnpm@10.28.2",
        pnpm: { onlyBuiltDependencies: ["esbuild", "vue-demi"] },
        scripts: { build: "nuxt build" },
        dependencies: {
          "@iconify-json/tabler": "1.2.35",
          "@nuxt/icon": "2.4.1",
          "@nuxt/ui": "4.9.0",
          "@yueli/ui": toFileSpecifier(tarball),
          nuxt: "4.4.8",
          tailwindcss: "4.3.1",
          vue: "3.5.39",
          "vue-router": "5.1.0",
        },
        devDependencies: { typescript: "6.0.3" },
      },
      null,
      2,
    )}\n`,
  );
  await writeFile(
    join(consumerRoot, "nuxt.config.ts"),
    `export default defineNuxtConfig({\n  modules: ["@nuxt/ui", "@yueli/ui"],\n  css: ["~/assets/css/main.css"],\n  devtools: { enabled: false },\n  ssr: true,\n  fonts: {\n    providers: { google: false, googleicons: false, bunny: false, fontshare: false, fontsource: false },\n  },\n});\n`,
  );
  await writeFile(
    join(consumerRoot, "app", "assets", "css", "main.css"),
    `@import "tailwindcss";\n@import "@nuxt/ui";\n@import "@yueli/ui/tailwind.css";\n@import "@yueli/ui/theme.css";\n`,
  );
  await writeFile(
    join(consumerRoot, "app", "app.config.ts"),
    `import { createUiPreset } from "@yueli/ui/theme";\n\nexport default defineAppConfig(createUiPreset({ primary: "blue" }));\n`,
  );
  await writeFile(
    join(consumerRoot, "app", "app.vue"),
    `<script setup lang="ts">
import { createCollectionRouteQueryCodec, createJsonCollectionQueryPolicy } from "@yueli/ui/collection";
import { PublicCommentThread } from "@yueli/ui/comments";
import type { PublicCommentMessages } from "@yueli/ui/comments";
import { CommentModerationCollection } from "@yueli/ui/comments/admin";
import type { CommentModerationCollectionActions, CommentModerationCollectionModel } from "@yueli/ui/comments/admin";
import type { AccountMenuAppearance, AccountMenuMessages } from "@yueli/ui/account-menu/pattern";
import { AdminRowActions, createAdminNavigationSearchItems, normalizeAdminNavigation } from "@yueli/ui/admin";
import type { AdminNavigationItem, AdminSearchGroup, AdminShellMessages } from "@yueli/ui/admin";
import { useVueCollectionWorkflow } from "@yueli/ui/collection/vue";
import { createVueRouterCollectionQuerySync } from "@yueli/ui/collection/vue-router";
import { useActionFeedback } from "@yueli/ui/feedback";
import { evaluateImageOptimization } from "@yueli/ui/image";
import { optimizeImageFile } from "@yueli/ui/image/browser";
import { ReadingTableOfContents } from "@yueli/ui/navigation/table-of-contents";
import { ContentShareActions } from "@yueli/ui/sharing/content-share";
import type { DashboardMessages } from "@yueli/ui/dashboard/pattern";
import type { RemoteSelectLoader, RemoteSelectMessages, RemoteSelectValue } from "@yueli/ui/remote-select";
import { publicUiManifest } from "@yueli/ui/manifest";
import { useVueSettingsWorkflow } from "@yueli/ui/settings/vue";

interface Query { q: string }
interface Item { id: string }

const queryPolicy = createJsonCollectionQueryPolicy<Query>();
const sync = createVueRouterCollectionQuerySync({
  router: useRouter(),
  codec: createCollectionRouteQueryCodec({
    q: { kind: "string", default: "", maxLength: 120 },
  }),
});
useVueCollectionWorkflow({
  initialQuery: { q: "" },
  queryPolicy,
  keyOf: (item: Item) => item.id,
  querySync: sync,
  load: async (_query, workflow) => {
    const token = workflow.beginLoad();
    workflow.resolveLoad(token, { items: [], total: 0 });
  },
});
const feedback = useActionFeedback({ resetMs: 0 });
const settingsForm = reactive({ title: "Packed" });
const settings = useVueSettingsWorkflow({
  snapshot: () => settingsForm,
  restore: (snapshot) => Object.assign(settingsForm, snapshot),
});
const dashboardMessages: DashboardMessages = {
  metrics: "Metrics",
  pending: { title: "Pending", description: "Needs attention" },
  recent: { title: "Recent", description: "Continue working" },
  health: { title: "Health", description: "Service status" },
  quickActions: { title: "Actions", description: "Next steps" },
};
const accountMessages: AccountMenuMessages = {
  currentUser: "Current user",
  logout: "Sign out",
  openMenu: (name) => "Open " + name + " menu",
};
const accountAppearance: AccountMenuAppearance = {
  value: "system",
  messages: { label: "Appearance", system: "System", light: "Light", dark: "Dark" },
  onChange: () => undefined,
};
const shellMessages: AdminShellMessages = {
  skipToContent: "Skip to content",
  search: "Search",
  searchPlaceholder: "Search pages",
};
const navigation = normalizeAdminNavigation([
  { label: "Dashboard", icon: "i-tabler-layout-dashboard", to: "/" },
  {
    label: "Settings",
    icon: "i-tabler-settings",
    type: "trigger",
    children: [
      { label: "Profile", to: "/?section=profile", active: true },
      { label: "Appearance", to: "/?section=appearance" },
    ],
  },
] satisfies readonly AdminNavigationItem[]);
const searchGroups: readonly AdminSearchGroup[] = [{ id: "pages", label: "Pages", items: createAdminNavigationSearchItems(navigation) }];
const remoteMessages: RemoteSelectMessages = {
  placeholder: "Select owner",
  searchPlaceholder: "Search owners",
  empty: "No owners",
  error: "Owners unavailable",
  retry: "Retry",
  minimumQuery: (count) => "Enter " + count + " characters",
};
const owner = ref<RemoteSelectValue | null>(null);
const packedSearch = ref("");
const loadOwners: RemoteSelectLoader = async ({ signal }) => {
  signal.throwIfAborted();
  return { items: [{ value: "packed", label: "Packed owner" }] };
};
const imageDecision = evaluateImageOptimization({ name: "packed.png", type: "image/png", size: 2_000_000 });
const readingHeadings = [{ id: "packed-heading", text: "Packed heading", level: 2 }];
const shareMessages = { weibo: "Weibo", x: "X", system: "Share", copy: "Copy", copied: "Copied", copyFailed: "Copy failed" };
const rowActions = [
  { id: "view", label: "View packed row", icon: "i-tabler-external-link", to: "/" },
  { id: "edit", label: "Edit packed row", icon: "i-tabler-pencil", onSelect: () => undefined },
];
const commentMessages: PublicCommentMessages = {
  count: (count) => count + " comments", replies: (count) => count + " replies",
  sort: "Comment order", loading: "Loading comments", oldest: "Oldest", newest: "Newest",
  reply: "Reply", cancelReply: "Cancel", anonymous: "Anonymous", empty: "No comments",
  closed: "Comments closed", loadError: "Comments unavailable", retry: "Retry",
  writeComment: "Write a comment", writeReply: "Write a reply", authorName: "Name",
  authorEmail: "Email", anonymousHint: "Anonymous comments are moderated", login: "Sign in",
  submit: "Comment", submitReply: "Reply", submitted: "Published", pending: "Pending",
  submitError: "Unable to comment", nameRequired: "Name is required",
};
const moderationModel: CommentModerationCollectionModel = {
  search: "", searchPlaceholder: "Search comments", items: [], state: "ready",
  total: 0, page: 1, pageSize: 20, sortOrder: "desc", lifecycle: "all",
};
const moderationActions: CommentModerationCollectionActions = {
  updateSearch: () => undefined, search: () => undefined,
  controlChange: () => undefined, clearFilters: () => undefined,
  retry: () => undefined, sort: () => undefined,
  lifecycleChange: () => undefined,
  pageChange: () => undefined, pageSizeChange: () => undefined,
};
void optimizeImageFile;
</script>

<template>
  <YAdminConsoleLayout brand-label="Packed admin" brand-icon="i-tabler-layout-dashboard" brand-to="/" :navigation="navigation" :search-groups="searchGroups" :messages="shellMessages" storage-key="packed-admin" main-id="main-content" back-to-top-label="Top">
    <template #account="{ collapsed }">
      <YAccountMenu name="Packed user" :messages="accountMessages" :appearance="accountAppearance" :trigger-mode="collapsed ? 'collapsed' : 'sidebar'" :logout="() => undefined" />
    </template>
    <YAdminPage id="packed" title="Dashboard" main-id="packed-content">
    <YRemoteSelect v-model="owner" :load="loadOwners" :messages="remoteMessages" />
    <YDashboardLayout title="Dashboard" :messages="dashboardMessages">
      <template #recent><span>Recent work</span></template>
    </YDashboardLayout>
    <YActionFeedbackButton :status="feedback.status.value" idle-label="Save" />
    <YCollectionFrame label="Packed collection" bulk-label="Bulk actions" :bulk-visible="true">
      <template #search="{ controlsId, controlsOpen, toggleControls }">
        <button type="button" :aria-controls="controlsId" :aria-expanded="controlsOpen" @click="toggleControls">Filters</button>
      </template>
      <template #controls><span>Controls</span></template>
      <template #bulk><span>Bulk</span></template>
      <template #columns><span>Name</span></template>
      <span>packed {{ publicUiManifest.length }} {{ imageDecision.reason }}</span>
      <template #footer><span>Footer</span></template>
    </YCollectionFrame>
    <YCollectionTableToolbar v-model:search="packedSearch" label="Packed table" search-placeholder="Search" search-action="Search" filter-label="Filters">
      <template #filters><span>Status</span></template>
      <template #utilities><button type="button">Columns</button></template>
      <template #selection><span>Selected</span></template>
    </YCollectionTableToolbar>
    <YBackToTop label="Back to top" :threshold="0" />
    <ReadingTableOfContents :items="readingHeadings" />
    <ContentShareActions title="Packed content" url="https://example.test/packed" :messages="shareMessages" />
    <AdminRowActions label="Packed row actions" :items="rowActions" />
    <PublicCommentThread :comments="[]" :total="0" :messages="commentMessages" :format-time="String" />
    <CommentModerationCollection :model="moderationModel" :actions="moderationActions" :format-date="String" />
    <h2 id="packed-heading">Packed heading</h2>
    <YSettingSection title="Settings"><input v-model="settingsForm.title" /></YSettingSection>
    </YAdminPage>
  </YAdminConsoleLayout>
</template>
`,
  );

  // The consumer lives under the OS temp directory, outside every workspace.
  // Passing pnpm's --ignore-workspace flag through npm 11 exits without a
  // diagnostic, so the isolated install needs no package-manager-specific flag.
  await runPnpm(["install"], { cwd: consumerRoot });
  await runPnpm(["build"], { cwd: consumerRoot });

  const assetRoot = join(consumerRoot, ".output", "public", "_nuxt");
  const generatedCss = await Promise.all(
    (await readdir(assetRoot))
      .filter((entry) => entry.endsWith(".css"))
      .map((entry) => readFile(join(assetRoot, entry), "utf8")),
  );
  if (!generatedCss.some((source) => source.includes("collection"))) {
    throw new Error(
      "Packed consumer CSS is missing CollectionFrame Tailwind selectors.",
    );
  }
  if (
    !generatedCss.some((source) =>
      source.includes("minmax(16rem,1.4fr)"),
    )
  ) {
    throw new Error(
      "Packed consumer CSS is missing CommentModerationCollection grid selectors.",
    );
  }
  if (
    !generatedCss.some((source) =>
      source.includes("@xl\\:grid-cols-\\[minmax\\(14rem\\,1fr\\)_auto\\]"),
    )
  ) {
    throw new Error(
      "Packed consumer CSS is missing CollectionTableToolbar responsive selectors.",
    );
  }
  if (!generatedCss.some((source) => source.includes("--yueli-surface-page"))) {
    throw new Error("Packed consumer CSS is missing public theme tokens.");
  }

  const packedPackage = JSON.parse(
    await readFile(
      join(consumerRoot, "node_modules", "@yueli", "ui", "package.json"),
      "utf8",
    ),
  );
  if (
    packedPackage.name !== "@yueli/ui" ||
    !packedPackage.exports?.["."] ||
    !packedPackage.exports?.["./account-menu/pattern"] ||
    !packedPackage.exports?.["./admin"] ||
    !packedPackage.exports?.["./tailwind.css"] ||
    !packedPackage.exports?.["./theme"] ||
    !packedPackage.exports?.["./theme.css"] ||
    !packedPackage.exports?.["./dashboard/pattern"] ||
    !packedPackage.exports?.["./feedback"] ||
    !packedPackage.exports?.["./feedback/pattern"] ||
    !packedPackage.exports?.["./image"] ||
    !packedPackage.exports?.["./image/browser"] ||
    !packedPackage.exports?.["./navigation/back-to-top"] ||
    !packedPackage.exports?.["./navigation/table-of-contents"] ||
    !packedPackage.exports?.["./sharing/content-share"] ||
    !packedPackage.exports?.["./remote-select"] ||
    !packedPackage.exports?.["./settings"] ||
    !packedPackage.exports?.["./settings/vue"] ||
    !packedPackage.exports?.["./settings/browser"] ||
    !packedPackage.exports?.["./settings/vue-router"] ||
    !packedPackage.exports?.["./settings/pattern"] ||
    !packedPackage.exports?.["./collection/pattern"] ||
    !packedPackage.exports?.["./collection/vue"] ||
    !packedPackage.exports?.["./collection/vue-router"]
  ) {
    throw new Error(
      "Installed tarball is missing the documented public exports.",
    );
  }
} finally {
  await rm(resolvedTemporaryRoot, { recursive: true, force: true });
}
