export interface SettingsBeforeUnloadOptions {
  readonly isDirty: () => boolean;
  readonly target?: Window;
}

export function bindSettingsBeforeUnload(
  options: SettingsBeforeUnloadOptions,
): () => void {
  const target =
    options.target ?? (typeof window === "undefined" ? undefined : window);
  if (!target) return () => undefined;

  function beforeUnload(event: BeforeUnloadEvent) {
    if (!options.isDirty()) return;
    event.preventDefault();
    event.returnValue = "";
  }

  target.addEventListener("beforeunload", beforeUnload);
  return () => target.removeEventListener("beforeunload", beforeUnload);
}
