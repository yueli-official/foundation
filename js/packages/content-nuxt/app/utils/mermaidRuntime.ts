type MermaidModule = (typeof import("mermaid"))["default"];

let modulePromise: Promise<MermaidModule> | null = null;
let initializedTheme: "dark" | "default" | null = null;
let renderLocked = false;
const renderWaiters: Array<() => void> = [];

function loadMermaid() {
  if (!modulePromise) {
    modulePromise = import("mermaid").then((module) => module.default);
  }
  return modulePromise;
}

// All NodeViews share one dynamic import and one explicit render lock. Mermaid
// also keeps global configuration, so concurrent consumers must be serialized.
async function acquireRenderLock() {
  if (!renderLocked) {
    renderLocked = true;
    return;
  }
  await new Promise<void>((resolve) => renderWaiters.push(resolve));
}

function releaseRenderLock() {
  const next = renderWaiters.shift();
  if (next) next();
  else renderLocked = false;
}

export async function renderMermaidSvg(
  id: string,
  code: string,
  theme: "dark" | "default",
) {
  await acquireRenderLock();
  try {
    const mermaid = await loadMermaid();
    if (initializedTheme !== theme) {
      mermaid.initialize({ startOnLoad: false, theme });
      initializedTheme = theme;
    }
    const result = await mermaid.render(id, code);
    // Mermaid resolves the caller before its own execution queue marks itself
    // idle. Yield one task so a render from a replacement NodeView cannot get
    // stranded behind the just-finished job.
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    return result;
  } finally {
    releaseRenderLock();
  }
}
