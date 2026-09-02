<script setup lang="ts">
/* eslint-disable vue/no-v-html */
// 阅读侧使用与编辑器相同的 Markdown 语义，渲染代码、KaTeX 和提示块。
// Mermaid 水合放在 .client 组件中，避免 Nitro 服务端制品包含 Mermaid。
interface ImagePreviewOptions {
  rendition: string;
  format?: "jpg" | "png" | "webp";
}

const props = defineProps<{
  content: string;
  imagePreview?: ImagePreviewOptions;
}>();

const { render } = useMarkdown();
const html = computed(() => render(props.content ?? ""));

const el = ref<HTMLElement>();
const previewOpen = ref(false);
const previewURL = ref("");
const previewAlt = ref("");

// 为普通代码块补充复制按钮。
function addCopyButtons() {
  if (!el.value) return;
  el.value.querySelectorAll("pre").forEach((pre) => {
    if (pre.querySelector(".copy-btn")) return;
    if (pre.querySelector("code.language-mermaid")) return;
    const btn = document.createElement("button");
    btn.className = "copy-btn";
    btn.type = "button";
    btn.textContent = "复制";
    btn.addEventListener("click", async () => {
      const code = pre.querySelector("code")?.textContent ?? "";
      await navigator.clipboard.writeText(code);
      btn.textContent = "已复制";
      setTimeout(() => {
        btn.textContent = "复制";
      }, 1500);
    });
    pre.style.position = "relative";
    pre.appendChild(btn);
  });
}

function prepareImages() {
  if (!el.value) return;
  el.value.querySelectorAll<HTMLImageElement>("img").forEach((image) => {
    image.loading = "lazy";
    image.decoding = "async";
    const target = props.imagePreview
      ? contentAssetRenditionURL(
          image.currentSrc || image.src,
          props.imagePreview.rendition,
          props.imagePreview.format,
        )
      : "";
    if (!target || image.closest("a")) return;
    image.dataset.contentPreviewUrl = target;
    image.classList.add("content-previewable-image");
    image.tabIndex = 0;
    image.setAttribute("role", "button");
    image.setAttribute(
      "aria-label",
      image.alt ? `查看大图：${image.alt}` : "查看大图",
    );
  });
}

function previewFromEvent(event: MouseEvent | KeyboardEvent) {
  if (
    event instanceof KeyboardEvent &&
    event.key !== "Enter" &&
    event.key !== " "
  )
    return;
  const image = (event.target as HTMLElement | null)?.closest<HTMLImageElement>(
    "img[data-content-preview-url]",
  );
  if (!image) return;
  event.preventDefault();
  previewURL.value = image.dataset.contentPreviewUrl || "";
  previewAlt.value = image.alt || "图片预览";
  previewOpen.value = Boolean(previewURL.value);
}

function postRender() {
  addCopyButtons();
  prepareImages();
}

watch(html, () => nextTick(postRender));
onMounted(() => nextTick(postRender));
</script>

<template>
  <!-- HTML 只来自 useMarkdown；多作者场景启用前必须在该边界加入净化器。 -->
  <div
    ref="el"
    class="prose content-prose max-w-none dark:prose-invert prose-headings:font-display prose-headings:tracking-tight prose-headings:scroll-mt-24 prose-a:text-primary prose-img:rounded-lg prose-pre:rounded-lg"
    v-html="html"
    @click="previewFromEvent"
    @keydown="previewFromEvent"
  />
  <ContentMermaidHydrator :target="el" />
  <UModal
    v-model:open="previewOpen"
    :title="previewAlt"
    :ui="{ content: 'sm:max-w-6xl', body: 'p-2 sm:p-3' }"
  >
    <template #body>
      <img
        :src="previewURL"
        :alt="previewAlt"
        class="mx-auto max-h-[80vh] max-w-full rounded-lg object-contain"
      />
    </template>
  </UModal>
</template>
