// Local draft auto-save for the post editor (editor E7). Persists title/content/
// excerpt to localStorage on an interval so an unsaved session survives a crash
// or accidental navigation; offers to restore on next open. Ported from donor,
// with the plugin options store dropped (auto-save is always on, 60s interval).

interface DraftData {
  title?: string
  content?: string
  excerpt?: string
  savedAt?: string
}

const AUTO_SAVE_INTERVAL = 60_000

export function useEditorDraft(
  formData: Ref<{ title?: string, content?: string, excerpt?: string }>,
  opts: {
    mode: 'create' | 'edit'
    entityId?: string | number
    keyPrefix?: string
    hasInitialContent: boolean
  },
) {
  const keyPrefix = opts.keyPrefix ?? 'blog:draft'

  const autoSaveKey = computed(() =>
    opts.mode === 'edit' && opts.entityId
      ? `${keyPrefix}:${opts.entityId}`
      : `${keyPrefix}:new`,
  )

  const lastAutoSaved = ref<Date | null>(null)
  const autoSavedLabel = computed(() => {
    if (!lastAutoSaved.value) return ''
    return `已自动保存 ${lastAutoSaved.value.toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
    })}`
  })

  const showDraftRestore = ref(false)
  const savedDraft = ref<DraftData | null>(null)
  const hasUnsavedChanges = ref(false)

  const doAutoSave = () => {
    if (!formData.value.title && !formData.value.content) return
    try {
      localStorage.setItem(
        autoSaveKey.value,
        JSON.stringify({
          title: formData.value.title,
          content: formData.value.content,
          excerpt: formData.value.excerpt,
          savedAt: new Date().toISOString(),
        }),
      )
      lastAutoSaved.value = new Date()
    } catch {}
  }

  const restoreDraft = () => {
    if (!savedDraft.value) return
    if (savedDraft.value.title) formData.value.title = savedDraft.value.title
    if (savedDraft.value.content) formData.value.content = savedDraft.value.content
    if (savedDraft.value.excerpt) formData.value.excerpt = savedDraft.value.excerpt
    showDraftRestore.value = false
    localStorage.removeItem(autoSaveKey.value)
  }

  const discardDraft = () => {
    localStorage.removeItem(autoSaveKey.value)
    showDraftRestore.value = false
  }

  const markSaved = () => {
    hasUnsavedChanges.value = false
    try {
      localStorage.removeItem(autoSaveKey.value)
    } catch {}
  }

  let autoSaveTimer: ReturnType<typeof setInterval>

  const startAutoSave = () => {
    // Check for an existing draft on mount (only when the server had no content,
    // so we never shadow a freshly-loaded post with a stale local copy).
    if (!opts.hasInitialContent) {
      try {
        const saved = localStorage.getItem(autoSaveKey.value)
        if (saved) {
          const draft = JSON.parse(saved)
          if (draft.title || draft.content) {
            savedDraft.value = draft
            showDraftRestore.value = true
          }
        }
      } catch {}
    }

    autoSaveTimer = setInterval(doAutoSave, AUTO_SAVE_INTERVAL)
  }

  onUnmounted(() => clearInterval(autoSaveTimer))

  return {
    autoSaveKey,
    autoSavedLabel,
    showDraftRestore,
    savedDraft,
    hasUnsavedChanges,
    restoreDraft,
    discardDraft,
    markSaved,
    startAutoSave,
  }
}
