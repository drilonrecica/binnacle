<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    eyebrow,
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
    /** Small uppercase label above the title (section eyebrow). */
    eyebrow?: string;
    description?: string;
    /** Id for the heading, so parents can label regions. */
    id?: string;
    /** Border tone for callouts; the content must state the condition in words. */
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
        {#if eyebrow}<span class="eyebrow">{eyebrow}</span>{/if}
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
    border: 1px solid var(--line);
    border-radius: var(--radius-card);
    background: var(--panel);
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
    padding: var(--card-pad) var(--card-pad-wide) 0;
  }
  .padded .card-header + .card-body {
    padding-top: var(--space-3);
  }
  .card:not(.padded) .card-header {
    padding-bottom: var(--card-pad);
    border-bottom: 1px solid var(--line);
  }
  .heading {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .eyebrow {
    color: var(--muted);
    font-size: var(--text-label);
    font-weight: var(--weight-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }
  h2 {
    font-family: var(--font-sans);
    font-size: var(--text-body);
    font-weight: var(--weight-semibold);
    letter-spacing: 0;
  }
  .danger h2 {
    color: var(--critical-fg);
  }
  .heading p {
    color: var(--muted);
    font-size: var(--text-meta);
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
    padding: var(--card-pad) var(--card-pad-wide);
  }
  .card-footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: var(--card-pad) var(--card-pad-wide);
    border-top: 1px solid var(--line);
    border-radius: 0 0 var(--radius-card) var(--radius-card);
    background: var(--panel-2);
  }
</style>
