// eslint-disable-next-line @typescript-eslint/triple-slash-reference -- Mammoth's browser bundle does not publish its own declaration file.
/// <reference path="../types/mammoth-browser.d.ts" />

import type { JSONContent } from "@tiptap/core";

export interface ImportFile {
  path: string;
  file: File;
}
export interface ImportDocument {
  name: string;
  format: "markdown" | "html";
  content: string;
  files: ImportFile[];
}

const MAX_FILE = 50 * 1024 * 1024;
const MAX_EXPANDED = 150 * 1024 * 1024;
const mimeTypes: Record<string, string> = {
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  webp: "image/webp",
  gif: "image/gif",
  avif: "image/avif",
  svg: "image/svg+xml",
  bmp: "image/bmp",
};

export function importPath(value: string): string {
  let path = value.replace(/\\/g, "/").split(/[?#]/)[0]!;
  try {
    path = decodeURIComponent(path);
  } catch {
    /* Keep literal filenames. */
  }
  const parts: string[] = [];
  for (const part of path.split("/")) {
    if (!part || part === ".") continue;
    if (part === "..") {
      if (!parts.length) throw new Error(`图片路径超出导入目录：${value}`);
      parts.pop();
    } else parts.push(part);
  }
  return parts.join("/");
}

export function imageFile(file: File): File {
  if (file.type.startsWith("image/")) return file;
  const type = mimeTypes[file.name.split(".").pop()?.toLowerCase() || ""];
  if (!type) throw new Error(`不是支持的图片：${file.name}`);
  return new File([file], file.name, { type });
}

async function archiveFiles(file: File): Promise<ImportFile[]> {
  const { unzip } = await import("fflate");
  let total = 0;
  let count = 0;
  let failure = "";
  const entries = await new Promise<Record<string, Uint8Array>>(
    (resolve, reject) => {
      void file
        .arrayBuffer()
        .then((buffer) =>
          unzip(
            new Uint8Array(buffer),
            {
              filter(entry) {
                total += entry.originalSize;
                if (
                  ++count > 500 ||
                  total > MAX_EXPANDED ||
                  entry.originalSize > MAX_FILE
                ) {
                  failure = "压缩包过大，请拆分后导入";
                  return false;
                }
                return !entry.name.endsWith("/");
              },
            },
            (error, data) => (error ? reject(error) : resolve(data)),
          ),
        )
        .catch(reject);
    },
  );
  if (failure) throw new Error(failure);
  return Object.entries(entries).map(([path, bytes]) => ({
    path: importPath(path),
    file: new File([bytes as Uint8Array<ArrayBuffer>], path.split("/").pop()!, {
      type: mimeTypes[path.split(".").pop()?.toLowerCase() || ""] || "",
    }),
  }));
}

export async function readImportDocument(
  files: File[],
): Promise<ImportDocument> {
  if (!files.length) throw new Error("请选择要导入的文档");
  const entries: ImportFile[] = [];
  for (const file of files) {
    if (file.size > MAX_FILE)
      throw new Error(`${file.name} 超过 50 MB，请拆分后导入`);
    if (/\.zip$/i.test(file.name)) entries.push(...(await archiveFiles(file)));
    else
      entries.push({
        path: importPath(file.webkitRelativePath || file.name),
        file,
      });
  }
  const documents = entries.filter((entry) =>
    /\.(md|markdown|html?|docx)$/i.test(entry.path),
  );
  if (documents.length !== 1)
    throw new Error(
      "请一次选择一份 Markdown、HTML 或 Word 文档，以及它引用的图片",
    );
  const document = documents[0]!;
  if (/\.docx$/i.test(document.path)) {
    // DOCX is also a ZIP: validate expanded sizes before the converter opens it.
    await archiveFiles(document.file);
    const { default: mammoth } = await import("mammoth/mammoth.browser.js");
    const result = await mammoth.convertToHtml({
      arrayBuffer: await document.file.arrayBuffer(),
    });
    return {
      name: document.path,
      format: "html",
      content: result.value,
      files: entries,
    };
  }
  return {
    name: document.path,
    format: /\.html?$/i.test(document.path) ? "html" : "markdown",
    content: await document.file.text(),
    files: entries,
  };
}

export async function cleanImportHTML(html: string): Promise<string> {
  const { default: purify } = await import("dompurify");
  return purify.sanitize(html, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: [
      "style",
      "script",
      "iframe",
      "object",
      "embed",
      "form",
      "input",
      "button",
      "video",
      "audio",
    ],
    FORBID_ATTR: ["srcset"],
  });
}

export function collectImportImages(document: JSONContent): JSONContent[] {
  const images: JSONContent[] = [];
  const walk = (node: JSONContent) => {
    if (node.type === "image") images.push(node);
    node.content?.forEach(walk);
  };
  walk(document);
  return images;
}

export function findImportImage(
  src: string,
  documentName: string,
  files: ImportFile[],
): File | undefined {
  const directory = documentName.includes("/")
    ? documentName.slice(0, documentName.lastIndexOf("/") + 1)
    : "";
  const path = importPath(directory + src);
  const exact = files.find((item) => item.path === path);
  if (exact) return exact.file;
  // Browsers expose basename-only selections. Accept only an unambiguous match.
  const name = importPath(src).split("/").pop();
  const matches = files.filter((item) => item.file.name === name);
  return matches.length === 1 ? matches[0]!.file : undefined;
}

export async function resolveImportImage(
  src: string,
  document: ImportDocument,
): Promise<File> {
  const local = findImportImage(src, document.name, document.files);
  if (local) return imageFile(local);
  if (/^data:image\//i.test(src)) {
    if (src.length > MAX_FILE * 1.4) throw new Error("内嵌图片过大");
    const blob = await (await fetch(src)).blob();
    const ext = blob.type.split("/")[1]?.replace("jpeg", "jpg") || "png";
    return imageFile(
      new File([blob], `imported-image.${ext}`, { type: blob.type }),
    );
  }
  if (/^https?:\/\//i.test(src)) {
    try {
      const response = await fetch(src, {
        credentials: "omit",
        signal: AbortSignal.timeout(30000),
      });
      if (!response.ok) throw new Error();
      if (Number(response.headers.get("content-length")) > MAX_FILE)
        throw new Error();
      const blob = await response.blob();
      if (blob.size > MAX_FILE || !blob.type.startsWith("image/"))
        throw new Error();
      return imageFile(
        new File(
          [blob],
          new URL(src).pathname.split("/").pop() || "image.png",
          { type: blob.type },
        ),
      );
    } catch {
      throw new Error(
        `无法读取图片：${src}。请下载图片后，与文档一起选择再导入。`,
      );
    }
  }
  throw new Error(`缺少图片：${src}。请补充该图片，或将文档和图片打包为 ZIP。`);
}

export async function uploadImportImages(
  tree: JSONContent,
  document: ImportDocument,
  uploader: ((file: File) => Promise<string>) | undefined,
  progress: (text: string) => void,
  cache = new Map<string, string>(),
): Promise<JSONContent> {
  const result = structuredClone(tree);
  const images = collectImportImages(result);
  if (images.length && !uploader)
    throw new Error("当前编辑器尚未接入图片上传，无法导入带图文档");
  const sources = [
    ...new Set(images.map((node) => String(node.attrs?.src || ""))),
  ];
  const resolved = new Map<string, File>();
  const failed = new Set<string>();
  // Resolve every reference first: a missing local image must not partially import the document.
  for (const src of sources) {
    progress(`正在读取图片 ${resolved.size + 1} / ${sources.length}`);
    if (!cache.has(src)) {
      try {
        resolved.set(src, await resolveImportImage(src, document));
      } catch (error) {
        if (!/^https?:\/\//i.test(src)) throw error;
        failed.add(src);
      }
    }
  }
  for (const [src, file] of resolved) {
    progress(`正在上传图片 ${cache.size + 1} / ${sources.length}`);
    try {
      cache.set(src, await uploader!(file));
    } catch (error) {
      if (!/^https?:\/\//i.test(src)) throw error;
      failed.add(src);
    }
  }
  const replace = (parent: JSONContent) => {
    parent.content = parent.content?.flatMap((node): JSONContent[] => {
      const src = String(node.attrs?.src || "");
      if (node.type === "image" && failed.has(src)) {
        const content: JSONContent[] = [
          {
            type: "text",
            text: `［图片未导入：${String(node.attrs?.alt || "外链图片")} · `,
          },
          {
            type: "text",
            text: "原图地址",
            marks: [{ type: "link", attrs: { href: src } }],
          },
          { type: "text", text: "］" },
        ];
        return parent.type === "paragraph" || parent.type === "heading"
          ? content
          : [{ type: "paragraph", content }];
      }
      if (node.type === "image")
        node.attrs = { ...node.attrs, src: cache.get(src) };
      else if (node.content) replace(node);
      return [node];
    });
  };
  replace(result);
  return result;
}
