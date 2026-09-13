import test from "node:test";
import assert from "node:assert/strict";
import { zipSync, strToU8 } from "fflate";
import {
  readImportDocument,
  uploadImportImages,
  findImportImage,
} from "../app/utils/editorImport.ts";

const image = new File(["image bytes"], "photo.png", { type: "image/png" });
const tree = (...sources) => ({
  type: "doc",
  content: sources.map((src) => ({
    type: "image",
    attrs: { src, alt: "说明" },
  })),
});

test("ZIP preserves Markdown-relative image paths and rejects ambiguous document selection", async () => {
  const data = zipSync({
    "article/post.md": strToU8("![图片](images/photo.png)"),
    "article/images/photo.png": strToU8("image"),
  });
  const doc = await readImportDocument([new File([data], "article.zip")]);
  assert.equal(doc.name, "article/post.md");
  assert.equal(
    findImportImage("images/photo.png", doc.name, doc.files)?.name,
    "photo.png",
  );
  await assert.rejects(
    readImportDocument([new File(["one"], "a.md"), new File(["two"], "b.md")]),
    /一次选择一份/,
  );
});

test("missing images abort before uploads and never mutate the source document", async () => {
  let uploaded = 0;
  const source = tree("photo.png", "missing.png");
  await assert.rejects(
    uploadImportImages(
      source,
      {
        name: "post.md",
        format: "markdown",
        content: "",
        files: [{ path: "photo.png", file: image }],
      },
      async () => {
        uploaded++;
        return "/media/test?v=1";
      },
      () => {},
    ),
    /缺少图片/,
  );
  assert.equal(uploaded, 0);
  assert.equal(source.content[0].attrs.src, "photo.png");
});

test("repeated references upload once and failed retries reuse completed uploads", async () => {
  const second = new File(["second"], "second.png", { type: "image/png" });
  const doc = {
    name: "post.md",
    format: "markdown",
    content: "",
    files: [
      { path: image.name, file: image },
      { path: second.name, file: second },
    ],
  };
  const source = tree("photo.png", "photo.png", "second.png");
  const cache = new Map();
  const calls = [];
  let fail = true;
  const upload = async (file) => {
    calls.push(file.name);
    if (file.name === "second.png" && fail) throw Error("上传失败");
    return `/media/${file.name}?v=1`;
  };
  await assert.rejects(
    uploadImportImages(source, doc, upload, () => {}, cache),
    /上传失败/,
  );
  fail = false;
  const result = await uploadImportImages(source, doc, upload, () => {}, cache);
  assert.deepEqual(calls, ["photo.png", "second.png", "second.png"]);
  assert.equal(result.content[0].attrs.src, result.content[1].attrs.src);
  assert.equal(result.content[2].attrs.src, "/media/second.png?v=1");
});

test("basename matching never chooses between two different files", () => {
  assert.equal(
    findImportImage("missing/photo.png", "post.md", [
      { path: "a/photo.png", file: image },
      { path: "b/photo.png", file: image },
    ]),
    undefined,
  );
});

test("DOCX converts document text and embedded image data", async () => {
  const files = {
    "[Content_Types].xml": strToU8(
      '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>',
    ),
    "_rels/.rels": strToU8(
      '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>',
    ),
    "word/document.xml": strToU8(
      '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body><w:p><w:r><w:t>Word 导入</w:t></w:r></w:p><w:p><w:r><w:drawing><wp:inline><a:graphic><a:graphicData><pic:pic><pic:blipFill><a:blip r:embed="rId2"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p></w:body></w:document>',
    ),
    "word/_rels/document.xml.rels": strToU8(
      '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/photo.png"/></Relationships>',
    ),
    "word/media/photo.png": new Uint8Array(await image.arrayBuffer()),
  };
  const result = await readImportDocument([
    new File([zipSync(files)], "document.docx"),
  ]);
  assert.match(result.content, /Word 导入/);
  assert.match(result.content, /data:image\/png;base64/);
});

test("failed remote images preserve text and continue other images", async (t) => {
  const calls = [];
  t.mock.method(globalThis, "fetch", async (src) => {
    calls.push(src);
    if (src.includes("network")) throw TypeError("fetch failed");
    if (src.includes("missing"))
      return new Response("missing", { status: 404 });
    return new Response("image", { headers: { "content-type": "image/png" } });
  });
  const source = tree(
    "https://example.com/missing.png",
    "https://example.com/network.png",
    "photo.png",
    "https://example.com/good.png",
    "https://example.com/missing.png",
  );
  source.content.unshift({
    type: "paragraph",
    content: [{ type: "text", text: "正文保留" }],
  });
  const uploaded = [];
  const result = await uploadImportImages(
    source,
    {
      name: "post.md",
      format: "markdown",
      content: "",
      files: [{ path: "photo.png", file: image }],
    },
    async (file) => {
      uploaded.push(file.name);
      return "/media/" + file.name;
    },
    () => {},
  );
  assert.deepEqual(uploaded, ["photo.png", "good.png"]);
  assert.equal(calls.length, 3);
  assert.equal(result.content[0].content[0].text, "正文保留");
  assert.match(result.content[1].content[0].text, /图片未导入/);
  assert.equal(
    result.content[1].content[1].marks[0].attrs.href,
    "https://example.com/missing.png",
  );
  assert.equal(result.content[3].attrs.src, "/media/photo.png");
  assert.equal(result.content[4].attrs.src, "/media/good.png");
  assert.equal(source.content[1].type, "image");
});
test("remote upload failure produces a valid inline placeholder", async (t) => {
  t.mock.method(
    globalThis,
    "fetch",
    async () =>
      new Response("image", { headers: { "content-type": "image/png" } }),
  );
  const result = await uploadImportImages(
    {
      type: "doc",
      content: [
        {
          type: "paragraph",
          content: [
            { type: "text", text: "before" },
            ...tree("https://example.com/photo.png").content,
            { type: "text", text: "after" },
          ],
        },
      ],
    },
    { name: "clipboard.html", format: "html", content: "", files: [] },
    async () => {
      throw Error("upload failed");
    },
    () => {},
  );
  assert.ok(result.content[0].content.every((node) => node.type === "text"));
  assert.equal(result.content[0].content.at(-1).text, "after");
});
