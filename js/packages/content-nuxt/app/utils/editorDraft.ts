export interface EditorDraftRecord<T extends Record<string, unknown>> {
  data: T;
  savedAt: string;
}

function normalized(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(normalized);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => [key, normalized(item)]),
  );
}

export function editorDraftSnapshot(value: Record<string, unknown>): string {
  return JSON.stringify(normalized(value));
}

export function serializeEditorDraft<T extends Record<string, unknown>>(
  data: T,
  savedAt = new Date().toISOString(),
): string {
  return JSON.stringify({ data, savedAt } satisfies EditorDraftRecord<T>);
}

export function parseEditorDraft<T extends Record<string, unknown>>(
  raw: string,
): EditorDraftRecord<T> | null {
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return null;
    }
    const savedAt = typeof parsed.savedAt === "string" ? parsed.savedAt : "";
    if (
      parsed.data &&
      typeof parsed.data === "object" &&
      !Array.isArray(parsed.data)
    ) {
      return { data: parsed.data as T, savedAt };
    }

    // Keep drafts written by the original flat schema restorable.
    const { savedAt: _savedAt, ...data } = parsed;
    if (!Object.keys(data).length) return null;
    return { data: data as T, savedAt };
  } catch {
    return null;
  }
}

export function shouldOfferEditorDraft<T extends Record<string, unknown>>(
  draft: EditorDraftRecord<T>,
  current: T,
): boolean {
  return editorDraftSnapshot(draft.data) !== editorDraftSnapshot(current);
}
