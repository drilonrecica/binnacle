import { createRawSnippet, type Snippet } from 'svelte';

/** A snippet rendering plain text, for children props in component tests. */
export function textSnippet(text: string): Snippet {
  return createRawSnippet(() => ({
    render: () =>
      `<span>${text.replace(/[&<>]/g, (c) => `&#${c.charCodeAt(0)};`)}</span>`,
  }));
}

/**
 * jsdom has no HTMLDialogElement.showModal or close and no scrollIntoView;
 * these stand-ins toggle `open`, fire `close` and do nothing on scroll, so the
 * components' own logic can be exercised.
 */
export function polyfillDom() {
  Element.prototype.scrollIntoView ??= function () {};
  const proto = HTMLDialogElement.prototype as HTMLDialogElement & {
    showModal: () => void;
  };
  if (typeof proto.showModal === 'function') return;
  proto.showModal = function (this: HTMLDialogElement) {
    this.setAttribute('open', '');
  };
  proto.close = function (this: HTMLDialogElement) {
    if (!this.hasAttribute('open')) return;
    this.removeAttribute('open');
    this.dispatchEvent(new Event('close'));
  };
}
