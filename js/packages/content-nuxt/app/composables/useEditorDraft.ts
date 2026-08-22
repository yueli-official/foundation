import {
  computed,
  onUnmounted,
  ref,
  shallowRef,
  toValue,
  watch,
  type MaybeRefOrGetter,
  type Ref,
} from "vue";
import {
  editorDraftSnapshot,
  parseEditorDraft,
  serializeEditorDraft,
  shouldOfferEditorDraft,
  type EditorDraftRecord,
} from "../utils/editorDraft";

const AUTO_SAVE_INTERVAL = 60_000;

export function useEditorDraft<T extends Record<string, unknown>>(
  formData: Ref<T>,
  opts: {
    mode: "create" | "edit";
    entityId?: MaybeRefOrGetter<string | number | undefined>;
    keyPrefix?: string;
    /** @deprecated Existing content is compared instead of suppressing recovery. */
    hasInitialContent?: boolean;
  },
) {
  const keyPrefix = opts.keyPrefix ?? "content:draft";
  const autoSaveKey = computed(() => {
    const entityId = toValue(opts.entityId);
    return opts.mode === "edit" && entityId
      ? `${keyPrefix}:${entityId}`
      : `${keyPrefix}:new`;
  });

  const lastAutoSaved = ref<Date | null>(null);
  const autoSavedLabel = computed(() => {
    if (!lastAutoSaved.value) return "";
    return `已自动保存 ${lastAutoSaved.value.toLocaleTimeString(undefined, {
      hour: "2-digit",
      minute: "2-digit",
    })}`;
  });
  const showDraftRestore = ref(false);
  const savedDraft = shallowRef<EditorDraftRecord<T> | null>(null);
  const savedSnapshot = ref(editorDraftSnapshot(formData.value));
  const hasUnsavedChanges = computed(
    () => editorDraftSnapshot(formData.value) !== savedSnapshot.value,
  );

  function removeStoredDraft() {
    try {
      localStorage.removeItem(autoSaveKey.value);
    } catch {
      // localStorage may be unavailable in restricted browser contexts.
    }
  }

  function loadStoredDraft() {
    showDraftRestore.value = false;
    savedDraft.value = null;
    try {
      const raw = localStorage.getItem(autoSaveKey.value);
      if (!raw) return;
      const draft = parseEditorDraft<T>(raw);
      if (!draft) {
        removeStoredDraft();
        return;
      }
      savedDraft.value = draft;
      showDraftRestore.value = shouldOfferEditorDraft(draft, formData.value);
      if (!showDraftRestore.value) {
        savedDraft.value = null;
        removeStoredDraft();
      }
    } catch {
      // Ignore inaccessible local drafts.
    }
  }

  function saveNow() {
    if (!hasUnsavedChanges.value) return false;
    try {
      const savedAt = new Date();
      localStorage.setItem(
        autoSaveKey.value,
        serializeEditorDraft(formData.value, savedAt.toISOString()),
      );
      lastAutoSaved.value = savedAt;
      return true;
    } catch {
      return false;
    }
  }

  function restoreDraft() {
    if (!savedDraft.value) return undefined;
    const restored = JSON.parse(JSON.stringify(savedDraft.value.data)) as T;
    formData.value = restored;
    showDraftRestore.value = false;
    savedDraft.value = null;
    removeStoredDraft();
    return restored;
  }

  function discardDraft() {
    removeStoredDraft();
    showDraftRestore.value = false;
    savedDraft.value = null;
  }

  function markSaved() {
    savedSnapshot.value = editorDraftSnapshot(formData.value);
    discardDraft();
  }

  let autoSaveTimer: ReturnType<typeof setInterval> | undefined;
  let started = false;
  function startAutoSave() {
    if (started) return;
    started = true;
    savedSnapshot.value = editorDraftSnapshot(formData.value);
    loadStoredDraft();
    autoSaveTimer = setInterval(saveNow, AUTO_SAVE_INTERVAL);
    window.addEventListener("pagehide", saveNow);
  }

  watch(autoSaveKey, (next, previous) => {
    if (started && next !== previous) loadStoredDraft();
  });

  onUnmounted(() => {
    saveNow();
    if (autoSaveTimer) clearInterval(autoSaveTimer);
    if (started) window.removeEventListener("pagehide", saveNow);
  });

  return {
    autoSaveKey,
    autoSavedLabel,
    showDraftRestore,
    savedDraft,
    hasUnsavedChanges,
    restoreDraft,
    discardDraft,
    markSaved,
    saveNow,
    startAutoSave,
  };
}
