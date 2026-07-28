<script setup lang="ts">
const props = defineProps<{ target?: HTMLElement }>();

const colorMode = useColorMode();
let mermaid: (typeof import("mermaid"))["default"] | null = null;

async function renderMermaid() {
  const target = props.target;
  if (!target) return;

  const targets: { host: Element; src: string }[] = [];
  target.querySelectorAll("code.language-mermaid").forEach((code) => {
    const pre = code.closest("pre");
    if (pre) targets.push({ host: pre, src: code.textContent ?? "" });
  });
  target
    .querySelectorAll<HTMLElement>(".mermaid-diagram[data-src]")
    .forEach((div) => {
      targets.push({
        host: div,
        src: decodeURIComponent(div.dataset.src || ""),
      });
    });
  if (!targets.length) return;

  if (!mermaid) mermaid = (await import("mermaid")).default;
  mermaid.initialize({
    startOnLoad: false,
    theme: colorMode.value === "dark" ? "dark" : "default",
  });

  for (const { host, src } of targets) {
    if (!src.trim()) continue;
    try {
      const id = `mermaid-${Math.random().toString(36).slice(2, 8)}`;
      const { svg } = await mermaid.render(id, src);
      const wrapper = document.createElement("div");
      wrapper.className = "mermaid-diagram";
      wrapper.dataset.src = encodeURIComponent(src);
      wrapper.innerHTML = svg;
      host.replaceWith(wrapper);
    } catch {
      // 渲染失败时保留源码块，避免内容无声消失。
    }
  }
}

watch(
  () => props.target,
  () => nextTick(renderMermaid),
  { immediate: true },
);
watch(
  () => colorMode.value,
  () => nextTick(renderMermaid),
);
</script>

<template>
  <span hidden />
</template>
