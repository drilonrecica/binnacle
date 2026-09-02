<script lang="ts">
  import CircleCheck from '@lucide/svelte/icons/circle-check';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Info from '@lucide/svelte/icons/info';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import X from '@lucide/svelte/icons/x';
  import { toasts } from './toast.svelte';
</script>

<div class="toaster" role="status" aria-live="polite" aria-atomic="false">
  {#each toasts.items as toast (toast.id)}
    <div class={`toast ${toast.tone}`}>
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
        aria-label="Dismiss"
        onclick={() => toasts.dismiss(toast.id)}
      >
        <X />
      </button>
    </div>
  {/each}
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
  .toast {
    display: grid;
    grid-template-columns: auto 1fr auto;
    gap: var(--space-3);
    align-items: start;
    padding: var(--space-3) var(--space-3) var(--space-3) var(--space-4);
    border: 1px solid var(--border-strong);
    border-left: 3px solid var(--toast-accent);
    border-radius: var(--radius);
    background: var(--surface);
    box-shadow: var(--shadow-lg);
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
    font-size: var(--text-sm);
  }
  .text p {
    color: var(--text-2);
  }
  .action {
    justify-self: start;
    margin-top: var(--space-1);
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent-text);
    font-size: var(--text-sm);
    font-weight: 500;
  }
  .dismiss {
    display: inline-flex;
    padding: 2px;
    border: 0;
    border-radius: 4px;
    background: none;
    color: var(--text-3);
  }
  .dismiss :global(svg) {
    width: 16px;
    height: 16px;
  }
  .dismiss:hover {
    color: var(--text);
    background: var(--surface-2);
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
