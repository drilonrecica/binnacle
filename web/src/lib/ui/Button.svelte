<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import Spinner from './Spinner.svelte';

  /**
   * primary: brass, one per view. secondary: outline. quiet: no chrome until
   * hover. danger: destructive. link: inline text action.
   */
  type Variant = 'primary' | 'secondary' | 'quiet' | 'danger' | 'link';
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
    border-radius: var(--radius-control);
    background: transparent;
    color: var(--fg);
    font-family: var(--font-sans);
    font-size: var(--text-body);
    font-weight: var(--weight-medium);
    line-height: 1;
    white-space: nowrap;
    text-decoration: none;
    transition:
      background var(--motion) var(--ease-out),
      border-color var(--motion) var(--ease-out),
      color var(--motion) var(--ease-out);
  }
  .btn:hover {
    text-decoration: none;
  }
  .btn.sm {
    height: var(--control-h-sm);
    padding: 0 var(--space-2);
    font-size: var(--text-meta);
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
    font-weight: var(--weight-semibold);
  }
  .primary:hover:not(:disabled) {
    background: var(--accent-hover);
    border-color: var(--accent-hover);
  }

  .secondary {
    border-color: var(--line-strong);
    background: transparent;
  }
  .secondary:hover:not(:disabled) {
    border-color: var(--muted);
    background: var(--panel-2);
  }

  .quiet {
    color: var(--muted);
  }
  .quiet:hover:not(:disabled) {
    background: var(--panel-2);
    color: var(--fg);
  }

  .danger {
    background: var(--critical-bg);
    border-color: var(--critical-border);
    color: var(--critical-fg);
  }
  .danger:hover:not(:disabled) {
    background: var(--critical-solid);
    border-color: var(--critical-solid);
    color: var(--fg-inverse);
  }

  .link {
    height: auto;
    padding: 0;
    color: var(--accent-text);
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
