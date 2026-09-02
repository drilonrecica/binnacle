<script lang="ts">
  import type { Snippet } from 'svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';

  let {
    title,
    description,
    dirty = false,
    saving = false,
    error = '',
    onsave,
    onreset,
    children,
    tone,
  }: {
    title: string;
    description?: string;
    dirty?: boolean;
    saving?: boolean;
    error?: string;
    onsave?: () => void;
    onreset?: () => void;
    children: Snippet;
    tone?: 'danger' | 'warn' | 'info';
  } = $props();
</script>

<Card {title} {description} {tone}>
  <form
    class="section"
    onsubmit={(event) => {
      event.preventDefault();
      onsave?.();
    }}
  >
    {@render children()}
    {#if error}<p class="error" role="alert">{error}</p>{/if}
  </form>
  {#snippet footer()}
    {#if onsave}
      {#if dirty}<span class="dirty">Unsaved changes</span>{/if}
      <Button variant="ghost" onclick={onreset} disabled={!dirty || saving}
        >Reset</Button
      >
      <Button
        variant="primary"
        onclick={onsave}
        disabled={!dirty}
        loading={saving}>Save changes</Button
      >
    {/if}
  {/snippet}
</Card>

<style>
  .section {
    display: grid;
    gap: var(--space-4);
  }
  .error {
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  .dirty {
    margin-right: auto;
    color: var(--warn-fg);
    font-size: var(--text-xs);
  }
</style>
