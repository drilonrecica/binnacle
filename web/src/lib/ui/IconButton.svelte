<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import { tooltip } from './tooltip';

  let {
    label,
    size = 'md',
    variant = 'ghost',
    href,
    children,
    class: className = '',
    ...rest
  }: {
    label: string;
    size?: 'sm' | 'md';
    variant?: 'ghost' | 'secondary';
    href?: string;
    children: Snippet;
    class?: string;
  } & HTMLButtonAttributes = $props();
</script>

{#if href}
  <a
    {href}
    class={`icon-btn ${size} ${variant} ${className}`}
    aria-label={label}
    use:tooltip={label}
    onclick={rest.onclick as never}
  >
    {@render children()}
  </a>
{:else}
  <button
    type="button"
    {...rest}
    class={`icon-btn ${size} ${variant} ${className}`}
    aria-label={label}
    use:tooltip={label}
  >
    {@render children()}
  </button>
{/if}

<style>
  .icon-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: var(--control-h);
    height: var(--control-h);
    padding: 0;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-2);
    transition:
      background var(--motion-fast) var(--ease),
      color var(--motion-fast) var(--ease);
  }
  .icon-btn.sm {
    width: var(--control-h-sm);
    height: var(--control-h-sm);
  }
  .icon-btn :global(svg) {
    width: 16px;
    height: 16px;
  }
  .icon-btn.sm :global(svg) {
    width: 14px;
    height: 14px;
  }
  .icon-btn:hover:not(:disabled) {
    background: var(--surface-2);
    color: var(--text);
    text-decoration: none;
  }
  .icon-btn.secondary {
    background: var(--surface);
    border-color: var(--border-strong);
    color: var(--text);
  }
  .icon-btn.secondary:hover:not(:disabled) {
    background: var(--surface-2);
  }
  .icon-btn:disabled {
    opacity: 0.5;
  }
</style>
