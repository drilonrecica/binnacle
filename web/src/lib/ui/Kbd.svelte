<script lang="ts">
  import type { Snippet } from 'svelte';
  let { children }: { children: Snippet } = $props();
</script>

<kbd>{@render children()}</kbd>

<style>
  kbd {
    font-family: var(--font-mono);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 20px;
    height: 20px;
    padding: 0 5px;
    border: 1px solid var(--line-strong);
    border-bottom-width: 2px;
    border-radius: var(--radius-control);
    background: var(--panel-2);
    color: var(--muted);
    font-size: var(--text-label);
    font-weight: var(--weight-medium);
    line-height: 1;
  }
</style>
