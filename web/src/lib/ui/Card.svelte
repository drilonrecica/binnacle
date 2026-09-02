<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    description,
    id,
    tone,
    padded = true,
    actions,
    footer,
    children,
    class: className = '',
  }: {
    title?: string;
    description?: string;
    /** Id for the heading, so parents can label regions. */
    id?: string;
    tone?: 'danger' | 'warn' | 'info';
    padded?: boolean;
    actions?: Snippet;
    footer?: Snippet;
    children?: Snippet;
    class?: string;
  } = $props();
</script>

<section
  class={`card ${tone ?? ''} ${className}`}
  class:padded
  aria-labelledby={title && id ? id : undefined}
>
  {#if title || actions}
    <header class="card-header">
      <div class="heading">
        {#if title}<h2 {id}>{title}</h2>{/if}
        {#if description}<p>{description}</p>{/if}
      </div>
      {#if actions}<div class="actions">{@render actions()}</div>{/if}
    </header>
  {/if}
  <div class="card-body">{@render children?.()}</div>
  {#if footer}<footer class="card-footer">{@render footer()}</footer>{/if}
</section>

<style>
  .card {
    display: flex;
    flex-direction: column;
    min-width: 0;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    box-shadow: var(--shadow-sm);
  }
  .card.danger {
    border-color: var(--critical-border);
  }
  .card.warn {
    border-color: var(--warn-border);
  }
  .card.info {
    border-color: var(--info-border);
  }
  .card-header {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3) var(--space-4);
    padding: var(--space-4) var(--space-4) 0;
  }
  .padded .card-header + .card-body {
    padding-top: var(--space-3);
  }
  .card:not(.padded) .card-header {
    padding-bottom: var(--space-3);
    border-bottom: 1px solid var(--border);
  }
  .heading {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  h2 {
    font-size: var(--text-md);
    font-weight: 600;
  }
  .danger h2 {
    color: var(--critical-fg);
  }
  .heading p {
    color: var(--text-2);
    font-size: var(--text-sm);
  }
  /* Grows to fill the rest of the row so wide toolbars wrap onto their own
     full-width line instead of pushing the page past the viewport. */
  .actions {
    display: flex;
    flex: 1 1 auto;
    flex-wrap: wrap;
    justify-content: flex-end;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }
  .card-body {
    min-width: 0;
    flex: 1;
  }
  .padded .card-body {
    padding: var(--space-4);
  }
  .card-footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border-top: 1px solid var(--border);
    background: var(--bg-subtle);
    border-radius: 0 0 var(--radius) var(--radius);
  }
</style>
