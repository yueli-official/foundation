import { generateJSON, type JSONContent, type Editor } from "@tiptap/vue-3";
import type { Transaction, Selection } from "@tiptap/pm/state";
import { computed, ref, type Ref } from "vue";
import {
  cleanImportHTML,
  collectImportImages,
  imageFile,
  readImportDocument,
  uploadImportImages,
  type ImportDocument,
} from "../utils/editorImport";

export function useEditorInput(
  editorRef: Ref<{ editor?: Editor } | null>,
  uploader: () => ((file: File) => Promise<string>) | undefined,
  reportError: (message: string) => void,
) {
  const imageOpen = ref(false);
  const importOpen = ref(false);
  const pending = ref(0);
  const progress = ref("");
  const uploading = computed(() => pending.value > 0);
  let tail = Promise.resolve();
  let importCache = new Map<string, string>();
  let importIdentity = "";

  function queue<T>(operation: () => Promise<T>): Promise<T> {
    pending.value++;
    const result = tail.then(operation);
    tail = result
      .then(
        () => {},
        () => {},
      )
      .finally(() => {
        pending.value--;
        if (!pending.value) progress.value = "";
      });
    return result;
  }

  function insertion(position?: number) {
    const editor = editorRef.value?.editor;
    if (!editor || !editor.isEditable) throw new Error("编辑器尚未准备好");
    let bookmark = editor.state.selection.getBookmark();
    if (position !== undefined) {
      const selectionType = editor.state.selection.constructor as unknown as {
        near: (position: Selection["$from"]) => Selection;
      };
      bookmark = selectionType
        .near(editor.state.doc.resolve(position))
        .getBookmark();
    }
    const map = ({ transaction }: { transaction: Transaction }) => {
      bookmark = bookmark.map(transaction.mapping);
    };
    editor.on("transaction", map);
    return {
      insert(content: JSONContent[]) {
        if (editor.isDestroyed)
          throw new Error("编辑器已关闭，请重新打开后导入");
        const selection = bookmark.resolve(editor.state.doc);
        editor
          .chain()
          .focus()
          .insertContentAt({ from: selection.from, to: selection.to }, content)
          .run();
      },
      release() {
        editor.off("transaction", map);
      },
    };
  }

  async function uploadImages(files: File[], _html = "", position?: number) {
    if (!files.length) throw new Error("请选择图片");
    const upload = uploader();
    if (!upload) throw new Error("当前编辑器尚未接入图片上传");
    const point = insertion(position);
    try {
      await queue(async () => {
        const nodes: JSONContent[] = [];
        for (const file of files) {
          progress.value = `正在上传图片 ${nodes.length + 1} / ${files.length}`;
          const src = await upload(imageFile(file));
          nodes.push({ type: "image", attrs: { src, alt: file.name } });
        }
        point.insert(nodes);
      });
    } finally {
      point.release();
    }
  }

  async function importDocument(files: File[], html: string) {
    const point = insertion();
    try {
      await queue(async () => {
        progress.value = "正在读取文档…";
        const document: ImportDocument = html
          ? {
              name: "clipboard.html",
              format: "html",
              content: html,
              files: files.map((file) => ({ path: file.name, file })),
            }
          : await readImportDocument(files);
        const identity = document.name + document.content;
        if (identity !== importIdentity) {
          importCache = new Map();
          importIdentity = identity;
        }
        const editor = editorRef.value?.editor;
        if (!editor) throw new Error("编辑器尚未准备好");
        let tree: JSONContent;
        if (document.format === "markdown") {
          const markdown = (
            editor as unknown as {
              markdown?: { parse: (text: string) => JSONContent };
            }
          ).markdown;
          if (!markdown) throw new Error("Markdown 解析器尚未准备好");
          tree = markdown.parse(document.content);
        } else {
          const html = new DOMParser().parseFromString(
            await cleanImportHTML(document.content),
            "text/html",
          );
          const sources = new Map<string, string>();
          for (const image of html.querySelectorAll("img")) {
            const placeholder = `/__editor_import_image__/${sources.size}`;
            sources.set(placeholder, image.getAttribute("src") || "");
            image.setAttribute("src", placeholder);
          }
          // The editor disallows persisted base64 images. Preserve import-only sources
          // while parsing its schema, then replace them through the upload adapter.
          tree = generateJSON(
            html.body.innerHTML,
            editor.extensionManager.extensions as never,
          );
          for (const image of collectImportImages(tree)) {
            const src = sources.get(String(image.attrs?.src || ""));
            if (src !== undefined) image.attrs = { ...image.attrs, src };
          }
        }
        const uploaded = await uploadImportImages(
          tree,
          document,
          uploader(),
          (text) => {
            progress.value = text;
          },
          importCache,
        );
        if (!uploaded.content?.length)
          throw new Error("文档中没有可导入的正文");
        point.insert(uploaded.content);
        importCache = new Map();
        importIdentity = "";
      });
    } finally {
      point.release();
    }
  }

  function paste(event: ClipboardEvent) {
    // Only intercept editor content. Dialogs and other form controls own their clipboard.
    if (!(event.target as HTMLElement)?.closest("[contenteditable=true]"))
      return;
    const files = Array.from(event.clipboardData?.files || []).filter((file) =>
      file.type.startsWith("image/"),
    );
    if (!files.length || !uploader()) return;
    event.preventDefault();
    event.stopPropagation();
    void uploadImages(files).catch((error) =>
      reportError(error instanceof Error ? error.message : "图片上传失败"),
    );
  }
  function drop(event: DragEvent) {
    const editor = editorRef.value?.editor;
    const files = Array.from(event.dataTransfer?.files || []).filter((file) =>
      file.type.startsWith("image/"),
    );
    if (!files.length || !editor || !uploader()) return;
    event.preventDefault();
    event.stopPropagation();
    const position = editor.view.posAtCoords({
      left: event.clientX,
      top: event.clientY,
    })?.pos;
    void uploadImages(files, "", position).catch((error) =>
      reportError(error instanceof Error ? error.message : "图片上传失败"),
    );
  }
  function dragOver(event: DragEvent) {
    if (
      uploader() &&
      Array.from(event.dataTransfer?.types || []).includes("Files")
    )
      event.preventDefault();
  }
  return {
    imageOpen,
    importOpen,
    uploading,
    progress,
    uploadImages,
    importDocument,
    paste,
    drop,
    dragOver,
  };
}
