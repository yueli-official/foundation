import { computed, shallowRef, triggerRef } from "vue";
import { SiteProfileEditor } from "./editor";
import type {
  SiteProfileFormSchema,
  SiteProfileReplaceResult,
  SiteProfileSnapshot,
} from "./types";

export function useSiteProfileEditor(
  schema: SiteProfileFormSchema,
  snapshot: SiteProfileSnapshot,
) {
  const editor = shallowRef(new SiteProfileEditor(schema, snapshot));
  const draft = computed(() => editor.value.draft);
  const dirty = computed(() => editor.value.dirty);

  function set(path: string, value: unknown) {
    editor.value.set(path, value);
    triggerRef(editor);
  }

  function reset() {
    editor.value.reset();
    triggerRef(editor);
  }

  function apply(result: SiteProfileReplaceResult) {
    editor.value.apply(result);
    triggerRef(editor);
  }

  return {
    editor,
    draft,
    dirty,
    field: (path: string) => editor.value.field(path),
    get: (path: string) => editor.value.get(path),
    set,
    reset,
    apply,
    request: () => editor.value.request(),
  };
}
