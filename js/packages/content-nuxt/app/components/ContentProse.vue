<script setup lang="ts">
/* eslint-disable vue/no-v-html */
// 阅读侧使用与编辑器相同的 Markdown 语义，渲染代码、KaTeX 和提示块。
// Mermaid 水合放在 .client 组件中，避免 Nitro 服务端制品包含 Mermaid。
const props = defineProps<{ content: string }>();

const { render } = useMarkdown();
const html = computed(() => render(props.content ?? ""));

const el = ref<HTMLElement>();

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

function postRender() {
  addCopyButtons();
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
  />
  <ContentMermaidHydrator :target="el" />
</template>
