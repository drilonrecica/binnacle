<script lang="ts">
  import CircleCheck from '@lucide/svelte/icons/circle-check';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Info from '@lucide/svelte/icons/info';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import X from '@lucide/svelte/icons/x';
  import { toasts, type ToastItem } from './toast.svelte';

  // Errors go to an assertive region and everything else to a polite one.
  // Both regions exist before any toast arrives, so screen readers announce
  // the first insertion too.
  const errors = $derived(toasts.items.filter((item) => item.tone === 'error'));
  const notices = $derived(
    toasts.items.filter((item) => item.tone !== 'error'),
  );

  function onFocusOut(event: FocusEvent, id: number) {
    const next = event.relatedTarget as Node | null;
    if (next && (event.currentTarget as HTMLElement).contains(next)) return;
    toasts.resume(id, 'focus');
  }
</script>

{#snippet toastView(toast: ToastItem)}
  <div
    class={`toast ${toast.tone}`}
    data-toast={toast.id}
    role="group"
    aria-label={toast.title}
    onmouseenter={() => toasts.pause(toast.id, 'hover')}
    onmouseleave={() => toasts.resume(toast.id, 'hover')}
    onfocusin={() => toasts.pause(toast.id, 'focus')}
    onfocusout={(event) => onFocusOut(event, toast.id)}
  >
    <span class="icon" aria-hidden="true">
      {#if toast.tone === 'success'}<CircleCheck
        />{:else if toast.tone === 'error'}<CircleAlert
        />{:else if toast.tone === 'warning'}<TriangleAlert />{:else}<Info
        />{/if}
    </span>
    <div class="text">
      <strong>{toast.title}</strong>
      {#if toast.description}<p>{toast.description}</p>{/if}
      {#if toast.action}
        <button
          type="button"
          class="action"
          onclick={() => {
            toast.action?.onclick();
            toasts.dismiss(toast.id);
          }}>{toast.action.label}</button
        >
      {/if}
    </div>
    <button
      type="button"
      class="dismiss"
      aria-label={`Dismiss: ${toast.title}`}
      onclick={() => toasts.dismiss(toast.id)}
    >
      <X aria-hidden="true" />
    </button>
  </div>
{/snippet}

<div class="toaster">
  <div class="region" role="alert" aria-live="assertive" aria-atomic="false">
    {#each errors as toast (toast.id)}{@render toastView(toast)}{/each}
  </div>
  <div class="region" role="status" aria-live="polite" aria-atomic="false">
    {#each notices as toast (toast.id)}{@render toastView(toast)}{/each}
  </div>
</div>

<style>
  .toaster {
    position: fixed;
    right: var(--space-4);
    bottom: var(--space-4);
    z-index: 100;
    display: grid;
    gap: var(--space-2);
    width: min(380px, calc(100vw - 2rem));
    pointer-events: none;
  }
  .region {
    display: grid;
    gap: var(--space-2);
  }
  .region:empty {
    display: none;
  }
  .toast {
    display: grid;
    grid-template-columns: auto 1fr auto;
    gap: var(--space-3);
    align-items: start;
    padding: var(--space-3) var(--space-3) var(--space-3) var(--space-4);
    border: 1px solid var(--line-strong);
    border-left: 3px solid var(--toast-accent);
    color: var(--fg);
    border-radius: var(--radius-card);
    background: var(--panel);
    box-shadow: var(--shadow-overlay);
    pointer-events: auto;
    animation: toast-in var(--motion) var(--ease-out);
  }
  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translateY(8px);
    }
  }
  .icon {
    display: inline-flex;
    color: var(--toast-accent);
    margin-top: 1px;
  }
  .icon :global(svg) {
    width: 18px;
    height: 18px;
  }
  .text {
    display: grid;
    gap: 2px;
    font-size: var(--text-body);
  }
  .text p {
    color: var(--muted);
  }
  .action {
    justify-self: start;
    margin-top: var(--space-1);
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent-text);
    font-size: var(--text-body);
    font-weight: var(--weight-medium);
  }
  .dismiss {
    display: inline-flex;
    padding: 2px;
    border: 0;
    border-radius: var(--radius-control);
    background: none;
    color: var(--muted);
  }
  .dismiss :global(svg) {
    width: 16px;
    height: 16px;
  }
  .dismiss:hover {
    color: var(--fg);
    background: var(--panel-2);
  }
  .success {
    --toast-accent: var(--ok-solid);
  }
  .error {
    --toast-accent: var(--critical-solid);
  }
  .warning {
    --toast-accent: var(--warn-solid);
  }
  .info {
    --toast-accent: var(--info-solid);
  }
  @media (max-width: 899px) {
    .toaster {
      right: var(--space-3);
      left: var(--space-3);
      bottom: calc(var(--tabbar-h) + var(--space-3));
      width: auto;
    }
  }
</style>
