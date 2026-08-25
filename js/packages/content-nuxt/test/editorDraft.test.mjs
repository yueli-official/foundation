import test from "node:test";
import assert from "node:assert/strict";

import {
  editorDraftStorageKey,
  parseEditorDraft,
  serializeEditorDraft,
  shouldOfferEditorDraft,
} from "../app/utils/editorDraft.ts";

test("create drafts can use a stable instance key while edit drafts keep entity keys", () => {
  assert.equal(
    editorDraftStorageKey("docs:doc", "create", "new-a"),
    "docs:doc:new:new-a",
  );
  assert.equal(
    editorDraftStorageKey("docs:doc", "create", "new-b"),
    "docs:doc:new:new-b",
  );
  assert.equal(
    editorDraftStorageKey("docs:doc", "edit", "doc-1"),
    "docs:doc:doc-1",
  );
  assert.equal(
    editorDraftStorageKey("docs:doc", "create"),
    "docs:doc:new",
  );
});

test("existing content still offers a different local draft without replacing it silently", () => {
  const current = { content: "server version" };
  const draft = parseEditorDraft(
    serializeEditorDraft({ content: "local version" }, "2026-08-21T10:00:00.000Z"),
  );

  assert.ok(draft);
  assert.equal(shouldOfferEditorDraft(draft, current), true);
  assert.deepEqual(current, { content: "server version" });
});

test("identical and malformed drafts do not interrupt the editor", () => {
  const current = { title: "Article", content: "same" };
  const draft = parseEditorDraft(serializeEditorDraft(current));

  assert.ok(draft);
  assert.equal(shouldOfferEditorDraft(draft, current), false);
  assert.equal(parseEditorDraft("not-json"), null);
});

test("legacy flat drafts remain restorable after the shared schema upgrade", () => {
  const draft = parseEditorDraft(
    JSON.stringify({ content: "legacy", savedAt: "2026-08-20T10:00:00.000Z" }),
  );

  assert.deepEqual(draft, {
    data: { content: "legacy" },
    savedAt: "2026-08-20T10:00:00.000Z",
  });
});
