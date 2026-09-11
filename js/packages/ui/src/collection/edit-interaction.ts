/** Row editing must not compete with selection, links, or inline controls. */
export function isCollectionEditGesture(event: MouseEvent): boolean {
  if (event.defaultPrevented || event.button !== 0) return false;
  const target = event.target;
  return target instanceof Element && !target.closest(
    'a, button, input, textarea, select, summary, [role="button"], [role="checkbox"], [contenteditable="true"]',
  );
}
