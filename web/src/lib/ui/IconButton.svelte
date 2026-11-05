<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import { tooltip } from './tooltip';

  // `label` is required: an icon-only control always has an accessible name,
  // and the same text shows as a tooltip on hover and focus.
  let {
    label,
    size = 'md',
    variant = 'quiet',
    href,
    children,
    class: className = '',
    ...rest
  }: {
    label: string;
    size?: 'sm' | 'md';
    variant?: 'quiet' | 'secondary';
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
    border-radius: var(--radius-control);
    background: transparent;
    color: var(--muted);
    transition:
      background var(--motion) var(--ease-out),
      color var(--motion) var(--ease-out);
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
    background: var(--panel-2);
    color: var(--fg);
    text-decoration: none;
  }
  .icon-btn.secondary {
    border-color: var(--line-strong);
    color: var(--fg);
  }
  .icon-btn:disabled {
    opacity: 0.5;
  }
</style>
