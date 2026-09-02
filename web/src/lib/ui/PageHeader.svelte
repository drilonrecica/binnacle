<script lang="ts">
  import type { Snippet } from 'svelte';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';

  let {
    title,
    description,
    crumbs = [],
    actions,
    meta,
    children,
    headingId = 'page-title',
  }: {
    title: string;
    description?: string;
    crumbs?: Array<{ label: string; href: string }>;
    actions?: Snippet;
    /** Inline meta row under the title (pills, timestamps). */
    meta?: Snippet;
    /** Extra content under the header, e.g. tabs or a toolbar. */
    children?: Snippet;
    headingId?: string;
  } = $props();
</script>

<header class="page-header">
  <div class="row">
    <div class="heading">
      {#if crumbs.length}
        <nav class="crumbs" aria-label="Breadcrumb">
          <ol>
            {#each crumbs as crumb (crumb.href)}
              <li>
                <a href={crumb.href}>{crumb.label}</a><ChevronRight
                  aria-hidden="true"
                />
              </li>
            {/each}
          </ol>
        </nav>
      {/if}
      <h1 id={headingId}>{title}</h1>
      {#if description}<p class="description">{description}</p>{/if}
      {#if meta}<div class="meta">{@render meta()}</div>{/if}
    </div>
    {#if actions}<div class="actions">{@render actions()}</div>{/if}
  </div>
  {@render children?.()}
</header>

<style>
  .page-header {
    display: grid;
    gap: var(--space-4);
    margin-bottom: var(--space-5);
  }
  .row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    flex-wrap: wrap;
  }
  .heading {
    display: grid;
    gap: var(--space-1);
    min-width: 0;
  }
  .crumbs ol {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    margin: 0;
    padding: 0;
    list-style: none;
    color: var(--text-3);
    font-size: var(--text-sm);
  }
  .crumbs li {
    display: inline-flex;
    align-items: center;
    gap: 2px;
  }
  .crumbs a {
    color: var(--text-2);
  }
  .crumbs :global(svg) {
    width: 14px;
    height: 14px;
    margin: 0 2px;
  }
  h1 {
    font-size: var(--text-xl);
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .description {
    color: var(--text-2);
    font-size: var(--text-sm);
    max-width: 72ch;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    margin-top: var(--space-1);
    color: var(--text-2);
    font-size: var(--text-sm);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
</style>
