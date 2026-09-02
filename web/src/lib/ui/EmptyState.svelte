<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    description,
    icon,
    tone = 'neutral',
    compact = false,
    children,
  }: {
    title: string;
    description?: string;
    icon?: Snippet;
    tone?: 'neutral' | 'ok';
    compact?: boolean;
    children?: Snippet;
  } = $props();
</script>

<div class={`empty ${tone}`} class:compact>
  {#if icon}<span class="icon" aria-hidden="true">{@render icon()}</span>{/if}
  <h3>{title}</h3>
  {#if description}<p>{description}</p>{/if}
  {#if children}<div class="actions">{@render children()}</div>{/if}
</div>

<style>
  .empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-8) var(--space-4);
    text-align: center;
  }
  .compact {
    padding: var(--space-4);
  }
  .icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    margin-bottom: var(--space-1);
    border-radius: var(--radius);
    background: var(--surface-2);
    color: var(--text-2);
  }
  .ok .icon {
    background: var(--ok-bg);
    color: var(--ok-fg);
  }
  .icon :global(svg) {
    width: 20px;
    height: 20px;
  }
  h3 {
    font-size: var(--text-md);
    font-weight: 600;
  }
  p {
    max-width: 44ch;
    color: var(--text-2);
    font-size: var(--text-sm);
  }
  .actions {
    display: flex;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }
</style>
