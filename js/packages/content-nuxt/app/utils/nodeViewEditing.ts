import { onBeforeUnmount, onMounted } from "vue";

const ACTIVATE_EVENT = "yueli:content-node-view-activate";
let nodeViewSequence = 0;

interface ExclusiveNodeViewEditingOptions {
  isEditing: () => boolean;
  close: () => void;
  focusSelector: string;
}

export function useExclusiveNodeViewEditing(
  options: ExclusiveNodeViewEditingOptions,
) {
  const nodeViewId = `content-node-view-${++nodeViewSequence}`;

  function focusSource() {
    const root = document.querySelector<HTMLElement>(
      `[data-yueli-node-view-id="${nodeViewId}"]`,
    );
    const target = root?.querySelector<HTMLElement>(options.focusSelector);
    target?.focus();
    return Boolean(target);
  }

  function activate() {
    if (!import.meta.client) return;
    window.dispatchEvent(
      new CustomEvent(ACTIVATE_EVENT, { detail: { id: nodeViewId } }),
    );
    requestAnimationFrame(() => {
      if (!focusSource()) setTimeout(focusSource, 0);
    });
  }

  function onOtherNodeActivated(event: Event) {
    const id = (event as CustomEvent<{ id?: string }>).detail?.id;
    if (id === nodeViewId || !options.isEditing()) return;
    options.close();
  }

  onMounted(() => {
    window.addEventListener(ACTIVATE_EVENT, onOtherNodeActivated);
    if (options.isEditing()) activate();
  });
  onBeforeUnmount(() => {
    window.removeEventListener(ACTIVATE_EVENT, onOtherNodeActivated);
  });

  return { nodeViewId, activate };
}
