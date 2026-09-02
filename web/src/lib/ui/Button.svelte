<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import Spinner from './Spinner.svelte';

  type Variant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'link';
  type Size = 'sm' | 'md';

  let {
    variant = 'secondary',
    size = 'md',
    loading = false,
    href,
    icon,
    iconOnly = false,
    class: className = '',
    children,
    disabled,
    ...rest
  }: {
    variant?: Variant;
    size?: Size;
    loading?: boolean;
    href?: string;
    icon?: Snippet;
    iconOnly?: boolean;
    class?: string;
    children?: Snippet;
  } & HTMLButtonAttributes = $props();
</script>

{#if href}
  <a
    {href}
    class={`btn ${variant} ${size} ${className}`}
    class:icon-only={iconOnly}
    aria-disabled={disabled || loading ? 'true' : undefined}
    onclick={rest.onclick as never}
  >
    {#if loading}<Spinner
        size={size === 'sm' ? 14 : 16}
      />{:else if icon}{@render icon()}{/if}
    {#if children}<span class="label">{@render children()}</span>{/if}
  </a>
{:else}
  <button
    type="button"
    {...rest}
    class={`btn ${variant} ${size} ${className}`}
    class:icon-only={iconOnly}
    disabled={disabled || loading}
    aria-busy={loading ? 'true' : undefined}
  >
    {#if loading}<Spinner
        size={size === 'sm' ? 14 : 16}
      />{:else if icon}{@render icon()}{/if}
    {#if children}<span class="label">{@render children()}</span>{/if}
  </button>
{/if}

<style>
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    height: var(--control-h);
    padding: 0 var(--space-3);
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text);
    font-size: var(--text-sm);
    font-weight: 500;
    line-height: 1;
    white-space: nowrap;
    text-decoration: none;
    transition:
      background var(--motion-fast) var(--ease),
      border-color var(--motion-fast) var(--ease),
      color var(--motion-fast) var(--ease),
      box-shadow var(--motion-fast) var(--ease);
  }
  .btn:hover {
    text-decoration: none;
  }
  .btn.sm {
    height: var(--control-h-sm);
    padding: 0 var(--space-2);
    font-size: var(--text-xs);
    gap: var(--space-1);
  }
  .btn.icon-only {
    width: var(--control-h);
    padding: 0;
  }
  .btn.icon-only.sm {
    width: var(--control-h-sm);
  }
  .btn :global(svg) {
    flex: none;
    width: 16px;
    height: 16px;
  }
  .btn.sm :global(svg) {
    width: 14px;
    height: 14px;
  }

  .primary {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-contrast);
    font-weight: 600;
  }
  .primary:hover:not(:disabled) {
    background: var(--accent-hover);
    border-color: var(--accent-hover);
  }

  .secondary {
    background: var(--surface);
    border-color: var(--border-strong);
    box-shadow: var(--shadow-sm);
  }
  .secondary:hover:not(:disabled) {
    background: var(--surface-2);
    border-color: var(--n-600);
  }

  .ghost {
    color: var(--text-2);
  }
  .ghost:hover:not(:disabled) {
    background: var(--surface-2);
    color: var(--text);
  }

  .danger {
    background: var(--critical-bg);
    border-color: var(--critical-border);
    color: var(--critical-fg);
  }
  .danger:hover:not(:disabled) {
    background: var(--critical-solid);
    border-color: var(--critical-solid);
    color: #fff;
  }

  .link {
    height: auto;
    padding: 0;
    color: var(--accent-text);
    font-weight: 500;
  }
  .link:hover:not(:disabled) {
    text-decoration: underline;
    text-underline-offset: 0.2em;
  }

  .btn:disabled,
  .btn[aria-disabled='true'] {
    opacity: 0.55;
    cursor: not-allowed;
    pointer-events: none;
  }
</style>
