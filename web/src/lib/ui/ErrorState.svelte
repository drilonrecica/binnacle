<script lang="ts">
  import type { Snippet } from 'svelte';
  import Button from './Button.svelte';
  import StatusMarker from './StatusMarker.svelte';

  let {
    title,
    description,
    onretry,
    retryLabel = 'Try again',
    compact = false,
    children,
  }: {
    /** What failed, e.g. "Incidents could not be loaded". */
    title: string;
    /** The error message or what the reader can do about it. */
    description?: string;
    onretry?: () => void;
    retryLabel?: string;
    compact?: boolean;
    children?: Snippet;
  } = $props();
</script>

<!-- A failed fetch is stated, never shown as an empty or healthy page. -->
<div class="error-state" class:compact role="alert">
  <p class="title">
    <StatusMarker tone="critical" size={9} />
    {title}
  </p>
  {#if description}<p class="description">{description}</p>{/if}
  {#if onretry || children}
    <div class="actions">
      {#if onretry}<Button size="sm" onclick={onretry}>{retryLabel}</Button
        >{/if}
      {@render children?.()}
    </div>
  {/if}
</div>

<style>
  .error-state {
    display: grid;
    justify-items: start;
    gap: var(--space-2);
    padding: var(--space-4);
    border: 1px solid var(--critical-border);
    border-radius: var(--radius-card);
    background: var(--critical-bg);
  }
  .compact {
    padding: var(--space-2) var(--space-3);
  }
  .title {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--critical-fg);
    font-weight: var(--weight-semibold);
  }
  .description {
    max-width: 72ch;
    color: var(--fg);
    font-size: var(--text-meta);
    overflow-wrap: anywhere;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
</style>
