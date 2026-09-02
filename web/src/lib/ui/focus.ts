const focusableSelector = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',');

export function focusableElements(root: HTMLElement): HTMLElement[] {
  return [...root.querySelectorAll<HTMLElement>(focusableSelector)].filter(
    (element) =>
      !element.hasAttribute('hidden') && element.offsetParent !== null,
  );
}

/** Keeps Tab focus inside `root` while attached. */
export function trapFocus(root: HTMLElement): () => void {
  const onKeydown = (event: KeyboardEvent) => {
    if (event.key !== 'Tab') return;
    const items = focusableElements(root);
    if (!items.length) {
      event.preventDefault();
      return;
    }
    const first = items[0];
    const last = items[items.length - 1];
    const active = document.activeElement as HTMLElement | null;
    if (event.shiftKey && (active === first || !root.contains(active))) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && active === last) {
      event.preventDefault();
      first.focus();
    }
  };
  root.addEventListener('keydown', onKeydown);
  return () => root.removeEventListener('keydown', onKeydown);
}

/** Svelte action: call `handler` when a pointer event lands outside the node. */
export function clickOutside(
  node: HTMLElement,
  handler: (event: PointerEvent) => void,
) {
  const listener = (event: PointerEvent) => {
    if (!node.contains(event.target as Node)) handler(event);
  };
  document.addEventListener('pointerdown', listener, true);
  return {
    update(next: (event: PointerEvent) => void) {
      handler = next;
    },
    destroy() {
      document.removeEventListener('pointerdown', listener, true);
    },
  };
}

let counter = 0;
export function uniqueId(prefix = 'ui'): string {
  counter += 1;
  return `${prefix}-${counter}`;
}
