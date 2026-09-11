<script setup lang="ts">
const props = defineProps<{
  mode: "images" | "document";
  submit: (files: File[], html: string) => Promise<void>;
  progress?: string;
}>();
const open = defineModel<boolean>("open", { default: false });
const files = ref<File[]>([]);
const html = ref("");
const busy = ref(false);
const error = ref("");
const dragging = ref(false);
const input = ref<HTMLInputElement>();
const previews = ref<string[]>([]);
const imageMode = computed(() => props.mode === "images");
function addFiles(incoming: File[]) {
  if (busy.value) return;
  error.value = "";
  const accepted = imageMode.value ? incoming.filter(file => file.type.startsWith("image/")) : incoming;
  if (!accepted.length) { error.value = "请选择图片文件"; return; }
  files.value = [...files.value, ...accepted];
}
function onFiles(event: Event) {
  const target = event.target as HTMLInputElement;
  addFiles(Array.from(target.files || []));
  target.value = "";
}
function onDrop(event: DragEvent) {
  event.preventDefault(); event.stopPropagation(); dragging.value = false;
  addFiles(Array.from(event.dataTransfer?.files || []));
}
function onPaste(event: ClipboardEvent) {
  const pasted = Array.from(event.clipboardData?.files || []);
  const rich = event.clipboardData?.getData("text/html") || "";
  if (!pasted.length && (!rich || imageMode.value)) return;
  event.preventDefault(); event.stopPropagation();
  if (busy.value) return;
  if (!imageMode.value && rich) { html.value = rich; error.value = ""; }
  if (pasted.length) addFiles(pasted);
}
watch(files, selected => {
  previews.value.forEach(url => URL.revokeObjectURL(url));
  previews.value = selected.map(file => file.type.startsWith("image/") ? URL.createObjectURL(file) : "");
});
watch(open, value => { if (!value) { files.value = []; html.value = ""; error.value = ""; } });
onBeforeUnmount(() => previews.value.forEach(url => URL.revokeObjectURL(url)));
async function confirm() {
  if (busy.value) return;
  busy.value = true; error.value = "";
  try { await props.submit(files.value, html.value); open.value = false; }
  catch (cause) { error.value = cause instanceof Error ? cause.message : "处理失败，请重试"; }
  finally { busy.value = false; }
}
</script>

<template>
  <UModal v-model:open="open" :title="imageMode ? '添加图片' : '导入文档'" :dismissible="!busy" :close="!busy" :ui="{ content: 'sm:max-w-xl' }">
    <template #body>
      <div class="grid gap-4" data-content-import-dialog @paste.capture="onPaste">
        <p class="text-sm text-muted">
          {{ imageMode ? '选择图片、拖拽到下方，或在此粘贴截图。' : '支持 Markdown、HTML、Word（.docx）和 ZIP，也可直接粘贴富文本。正文会插入光标位置。' }}
        </p>
        <button type="button" class="grid min-h-32 w-full place-content-center gap-2 rounded-lg border border-dashed border-default px-4 py-6 text-center transition-colors hover:bg-elevated focus-visible:outline-2 focus-visible:outline-primary" :class="{ 'bg-elevated border-primary': dragging }" :disabled="busy" :aria-label="imageMode ? '选择或拖入图片' : '选择文档及图片'" @click="input?.click()" @dragover.prevent="dragging = true" @dragleave="dragging = false" @drop="onDrop">
          <UIcon :name="imageMode ? 'i-tabler-photo-plus' : 'i-tabler-file-import'" class="mx-auto size-6 text-muted" />
          <span class="text-sm font-medium">{{ imageMode ? '选择图片或拖拽到这里' : '选择文档及图片，或拖入 ZIP' }}</span>
        </button>
        <input ref="input" type="file" multiple :accept="imageMode ? 'image/*' : '.md,.markdown,.html,.htm,.docx,.zip,image/*'" class="hidden" data-content-import-input @change="onFiles" />
        <p v-if="!imageMode" class="text-xs text-muted">本地图片请与文档一起选择，或保留目录结构打包为 ZIP。图片处理成功后才插入正文。</p>
        <div v-if="html" class="flex items-center justify-between gap-2 text-sm">
          <span>已粘贴富文本内容</span><UButton label="移除" variant="ghost" color="neutral" size="xs" :disabled="busy" @click="() => { html = ''; }" />
        </div>
        <ul v-if="files.length" class="grid max-h-52 gap-2 overflow-y-auto" aria-label="待处理文件">
          <li v-for="(file, index) in files" :key="index" class="flex min-w-0 items-center gap-2 rounded-lg border border-default p-2">
            <img v-if="previews[index]" :src="previews[index]" alt="" class="size-9 shrink-0 rounded object-cover" />
            <UIcon v-else name="i-tabler-file-text" class="size-5 shrink-0 text-muted" />
            <span class="min-w-0 flex-1 truncate text-sm">{{ file.name }}</span>
            <UButton icon="i-tabler-x" :aria-label="`移除 ${file.name}`" variant="ghost" color="neutral" size="xs" :disabled="busy" @click="() => { files = files.filter((_, i) => i !== index); }" />
          </li>
        </ul>
        <p v-if="busy" role="status" class="text-sm text-muted">{{ progress || '正在处理…' }}</p>
        <p v-if="error" role="alert" class="break-words text-sm text-error">{{ error }}</p>
      </div>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="取消" variant="outline" color="neutral" :disabled="busy" @click="() => { open = false; }" />
        <UButton :label="imageMode ? '上传并插入' : '导入到正文'" :loading="busy" :disabled="!files.length && !html" @click="confirm" />
      </div>
    </template>
  </UModal>
</template>
