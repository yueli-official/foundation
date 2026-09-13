import test from "node:test";
import assert from "node:assert/strict";
import { Editor } from "@tiptap/core";
import { Image } from "@tiptap/extension-image";
import { TableKit } from "@tiptap/extension-table";
import { Markdown } from "@tiptap/markdown";
import StarterKit from "@tiptap/starter-kit";
import { LinkedImage } from "../app/extensions/LinkedImage.ts";

const legacyMotionTable = `|  |  |
| --- | --- |
| ![](/images/2020/motion-periodic-table/delaymove.gif) | DelayMove 延迟移动 |

## 应用

|  |  |  |  |  |
| --- | --- | --- | --- | --- |
| [![](/images/2020/motion-periodic-table/delaymove.gif)](./repeattrim.md) | ![](/images/2020/motion-periodic-table/plus.png) | [![](/images/2020/motion-periodic-table/wigglemove.gif)](./wigglemove.md) | ![](/images/2020/motion-periodic-table/tri.png) | ![](/images/2020/motion-periodic-table/delaymove-ex001.gif) |

## 例

![](/images/2020/motion-periodic-table/delaymove-ex001.gif)

## 相似/相关

|  |  |
| --- | --- |
| [![](/images/2020/motion-periodic-table/motionblur.gif)](./motionblur.md) | [![](/images/2020/motion-periodic-table/wigglemove.gif)](./wigglemove.md) |`;

test("rich editor preserves legacy markdown tables, images and linked images", () => {
  const editor = new Editor({
    content: legacyMotionTable,
    contentType: "markdown",
    extensions: [
      Markdown.configure({ markedOptions: { gfm: true } }),
      StarterKit,
      Image,
      TableKit,
      LinkedImage,
    ],
  });

  try {
    const markdown = editor.getMarkdown();
    const json = JSON.stringify(editor.getJSON());

    assert.ok(json.includes('"type":"table"'));
    assert.ok(json.includes('"type":"linkedImage"'));
    assert.match(markdown, /delaymove-ex001\.gif/u);
    assert.match(markdown, /plus\.png/u);
    assert.ok(
      markdown.includes(
        "[![](/images/2020/motion-periodic-table/delaymove.gif)](./repeattrim.md)",
      ),
    );
    assert.ok(
      markdown.includes(
        "[![](/images/2020/motion-periodic-table/wigglemove.gif)](./wigglemove.md)",
      ),
    );
    assert.ok(
      markdown.includes(
        "[![](/images/2020/motion-periodic-table/motionblur.gif)](./motionblur.md)",
      ),
    );
  } finally {
    editor.destroy();
  }
});
