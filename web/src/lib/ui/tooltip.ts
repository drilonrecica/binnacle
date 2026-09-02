import { attachFloating } from './floating';
import { uniqueId } from './focus';

/**
 * Svelte action that shows a small text tooltip on hover and keyboard focus.
 * The tooltip is appended to `document.body` and referenced through
 * `aria-describedby`, so it is announced without being in the tab order.
 */
export function tooltip(node: HTMLElement, text: string | undefined | null) {
  let element: HTMLDivElement | null = null;
  let cleanup: (() => void) | null = null;
  let current = text;
  const id = uniqueId('tooltip');

  function show() {
    if (!current || element) return;
    element = document.createElement('div');
    element.className = 'ui-tooltip';
    element.setAttribute('role', 'tooltip');
    element.id = id;
    element.textContent = current;
    document.body.append(element);
    node.setAttribute('aria-describedby', id);
    cleanup = attachFloating(node, element, { placement: 'top', gap: 8 });
  }

  function hide() {
    cleanup?.();
    cleanup = null;
    element?.remove();
    element = null;
    if (node.getAttribute('aria-describedby') === id)
      node.removeAttribute('aria-describedby');
  }

  const onKeydown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') hide();
  };

  node.addEventListener('mouseenter', show);
  node.addEventListener('mouseleave', hide);
  node.addEventListener('focus', show);
  node.addEventListener('blur', hide);
  node.addEventListener('keydown', onKeydown);

  return {
    update(next: string | undefined | null) {
      current = next;
      if (element) {
        if (next) element.textContent = next;
        else hide();
      }
    },
    destroy() {
      hide();
      node.removeEventListener('mouseenter', show);
      node.removeEventListener('mouseleave', hide);
      node.removeEventListener('focus', show);
      node.removeEventListener('blur', hide);
      node.removeEventListener('keydown', onKeydown);
    },
  };
}
