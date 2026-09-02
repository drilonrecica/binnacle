<script lang="ts">
  import { getContext, type Snippet } from 'svelte';
  import { menuContextKey, type MenuContext } from './Menu.svelte';

  let {
    onselect,
    href,
    danger = false,
    disabled = false,
    icon,
    children,
  }: {
    onselect?: () => void;
    href?: string;
    danger?: boolean;
    disabled?: boolean;
    icon?: Snippet;
    children: Snippet;
  } = $props();

  const menu = getContext<MenuContext>(menuContextKey);

  function activate(event: MouseEvent) {
    if (disabled) {
      event.preventDefault();
      return;
    }
    menu.close();
    onselect?.();
  }
</script>

{#if href}
  <a
    {href}
    class="item"
    class:danger
    role="menuitem"
    tabindex="-1"
    onclick={activate}
  >
    {#if icon}<span class="icon">{@render icon()}</span>{/if}
    <span>{@render children()}</span>
  </a>
{:else}
  <button
    type="button"
    class="item"
    class:danger
    role="menuitem"
    tabindex="-1"
    {disabled}
    onclick={activate}
  >
    {#if icon}<span class="icon">{@render icon()}</span>{/if}
    <span>{@render children()}</span>
  </button>
{/if}

<style>
  .item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    min-height: 32px;
    padding: 0 var(--space-2);
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text);
    font-size: var(--text-sm);
    text-align: left;
    text-decoration: none;
  }
  .item:hover,
  .item:focus-visible {
    background: var(--surface-2);
    outline: none;
    text-decoration: none;
  }
  .item:disabled {
    opacity: 0.5;
  }
  .item.danger {
    color: var(--critical-fg);
  }
  .icon {
    display: inline-flex;
    color: var(--text-2);
  }
  .icon :global(svg) {
    width: 15px;
    height: 15px;
  }
  .item.danger .icon {
    color: inherit;
  }
</style>
