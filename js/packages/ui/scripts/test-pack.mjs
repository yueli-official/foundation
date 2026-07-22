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
const resolvedTemporaryRoot = resolve(temporaryRoot);
const pnpmEntry = process.env.npm_execpath;

if (
  dirname(resolvedTemporaryRoot) !== resolve(tmpdir()) ||
  !basename(resolvedTemporaryRoot).startsWith("yueli-ui-pack-")
) {
  throw new Error(
    `Refusing to use unexpected temporary directory: ${resolvedTemporaryRoot}`,
  );
}
if (!pnpmEntry)
  throw new Error(
    "Run this conformance through pnpm so npm_execpath is available.",
  );

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
  if (pnpmEntry.toLowerCase().endsWith(".exe"))
    return run(pnpmEntry, args, options);
  return run(process.execPath, [pnpmEntry, ...args], options);
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
    "package/src/collection/components/CollectionFrame.vue",
    "package/src/collection/components/CollectionPanel.vue",
    "package/src/collection/index.ts",
    "package/src/collection/panel.ts",
    "package/src/collection/pattern.ts",
    "package/src/collection/route-query.ts",
    "package/src/collection/vue.ts",
    "package/src/collection/vue-router.ts",
    "package/src/collection/workflow.ts",
    "package/src/feedback/action.ts",
    "package/src/feedback/components/ActionFeedbackButton.vue",
    "package/src/feedback/index.ts",
    "package/src/feedback/minimum-loading.ts",
    "package/src/feedback/notice.ts",
    "package/src/feedback/pattern.ts",
    "package/src/manifest.ts",
    "package/src/messages.ts",
    "package/src/module.ts",
    "package/src/navigation/back-to-top.ts",
    "package/src/navigation/components/BackToTop.vue",
    "package/src/tailwind.css",
  ].sort();
  if (JSON.stringify(packedFiles) !== JSON.stringify(allowedFiles)) {
    throw new Error(`Unexpected tarball contents:\n${packedFiles.join("\n")}`);
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
    `@import "tailwindcss";\n@import "@nuxt/ui";\n@import "@yueli/ui/tailwind.css";\n`,
  );
  await writeFile(
    join(consumerRoot, "app", "app.vue"),
    `<script setup lang="ts">
import { createCollectionRouteQueryCodec, createJsonCollectionQueryPolicy } from "@yueli/ui/collection";
import { useVueCollectionWorkflow } from "@yueli/ui/collection/vue";
import { createVueRouterCollectionQuerySync } from "@yueli/ui/collection/vue-router";
import { useActionFeedback } from "@yueli/ui/feedback";
import { publicUiManifest } from "@yueli/ui/manifest";

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
</script>

<template>
  <main id="main-content" tabindex="-1">
    <YActionFeedbackButton :status="feedback.status.value" idle-label="Save" />
    <YCollectionFrame label="Packed collection" bulk-label="Bulk actions" :bulk-visible="true">
      <template #search="{ controlsId, controlsOpen, toggleControls }">
        <button type="button" :aria-controls="controlsId" :aria-expanded="controlsOpen" @click="toggleControls">Filters</button>
      </template>
      <template #controls><span>Controls</span></template>
      <template #bulk><span>Bulk</span></template>
      <template #columns><span>Name</span></template>
      <span>packed {{ publicUiManifest.length }}</span>
      <template #footer><span>Footer</span></template>
    </YCollectionFrame>
    <YBackToTop label="Back to top" :threshold="0" />
  </main>
</template>
`,
  );

  await runPnpm(["install", "--ignore-workspace"], { cwd: consumerRoot });
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

  const packedPackage = JSON.parse(
    await readFile(
      join(consumerRoot, "node_modules", "@yueli", "ui", "package.json"),
      "utf8",
    ),
  );
  if (
    packedPackage.name !== "@yueli/ui" ||
    !packedPackage.exports?.["."] ||
    !packedPackage.exports?.["./tailwind.css"] ||
    !packedPackage.exports?.["./feedback"] ||
    !packedPackage.exports?.["./feedback/pattern"] ||
    !packedPackage.exports?.["./navigation/back-to-top"] ||
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
