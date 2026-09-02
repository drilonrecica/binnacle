<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import X from '@lucide/svelte/icons/x';
  import IconButton from './IconButton.svelte';
  import { focusableElements, trapFocus, uniqueId } from './focus';

  let {
    open = $bindable(false),
    title,
    description,
    size = 'md',
    dismissible = true,
    onclose,
    children,
    footer,
  }: {
    open?: boolean;
    title: string;
    description?: string;
    size?: 'sm' | 'md' | 'lg';
    /** When false, Escape and backdrop clicks do not close the dialog. */
    dismissible?: boolean;
    onclose?: () => void;
    children?: Snippet;
    footer?: Snippet;
  } = $props();

  const titleId = uniqueId('dialog-title');
  const descriptionId = uniqueId('dialog-description');
  let element = $state<HTMLDialogElement | null>(null);
  let opener: HTMLElement | null = null;
  let releaseTrap: (() => void) | null = null;

  $effect(() => {
    if (!element) return;
    if (open && !element.open) {
      opener = document.activeElement as HTMLElement | null;
      element.showModal();
      releaseTrap = trapFocus(element);
      void tick().then(() => {
        const preferred =
          element?.querySelector<HTMLElement>('[data-autofocus]');
        (
          preferred ??
          focusableElements(element!).find((el) => !el.closest('.dialog-close'))
        )?.focus();
      });
    } else if (!open && element.open) {
      element.close();
    }
  });

  function handleClose() {
    releaseTrap?.();
    releaseTrap = null;
    open = false;
    onclose?.();
    opener?.focus();
  }

  function handleCancel(event: Event) {
    if (!dismissible) {
      event.preventDefault();
      return;
    }
  }

  function handleBackdrop(event: MouseEvent) {
    if (!dismissible || event.target !== element) return;
    element?.close();
  }
</script>

<dialog
  bind:this={element}
  class={`dialog ${size}`}
  aria-labelledby={titleId}
  aria-describedby={description ? descriptionId : undefined}
  onclose={handleClose}
  oncancel={handleCancel}
  onclick={handleBackdrop}
>
  {#if open}
    <div class="panel">
      <header class="header">
        <div class="heading">
          <h2 id={titleId}>{title}</h2>
          {#if description}<p id={descriptionId}>{description}</p>{/if}
        </div>
        {#if dismissible}
          <span class="dialog-close">
            <IconButton label="Close" onclick={() => element?.close()}
              ><X /></IconButton
            >
          </span>
        {/if}
      </header>
      <div class="body">{@render children?.()}</div>
      {#if footer}<footer class="footer">{@render footer()}</footer>{/if}
    </div>
  {/if}
</dialog>

<style>
  .dialog {
    width: min(100vw - 2rem, var(--dialog-w));
    max-height: min(100vh - 2rem, 88vh);
    padding: 0;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    background: var(--surface);
    color: var(--text);
    box-shadow: var(--shadow-lg);
    overflow: hidden;
  }
  .dialog.sm {
    --dialog-w: 420px;
  }
  .dialog.md {
    --dialog-w: 560px;
  }
  .dialog.lg {
    --dialog-w: 760px;
  }
  .dialog[open] {
    animation: dialog-in var(--motion) var(--ease-out);
  }
  .dialog::backdrop {
    background: var(--overlay);
    backdrop-filter: blur(2px);
  }
  @keyframes dialog-in {
    from {
      opacity: 0;
      transform: translateY(8px) scale(0.98);
    }
  }
  .panel {
    display: flex;
    flex-direction: column;
    max-height: min(100vh - 2rem, 88vh);
  }
  .header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-5) var(--space-5) var(--space-3);
  }
  .heading {
    display: grid;
    gap: var(--space-1);
  }
  h2 {
    font-size: var(--text-lg);
  }
  .heading p {
    color: var(--text-2);
    font-size: var(--text-sm);
  }
  .body {
    padding: 0 var(--space-5) var(--space-5);
    overflow: auto;
  }
  .footer {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-5);
    border-top: 1px solid var(--border);
    background: var(--bg-subtle);
  }
  @media (max-width: 600px) {
    .dialog {
      width: calc(100vw - 1rem);
      max-height: calc(100vh - 1rem);
    }
  }
</style>
