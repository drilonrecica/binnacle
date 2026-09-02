<script lang="ts">
  import CircleCheck from '@lucide/svelte/icons/circle-check';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import type { AttentionItem } from './attention';
  import RelativeTime from '../ui/RelativeTime.svelte';

  let { items, limit = 6 }: { items: AttentionItem[]; limit?: number } =
    $props();
  const shown = $derived(items.slice(0, limit));
  const overflow = $derived(items.length - shown.length);
  const worst = $derived(
    items.some((item) => item.tone === 'critical')
      ? 'critical'
      : items.length
        ? 'warn'
        : 'ok',
  );
</script>

{#if items.length}
  <section class={`strip ${worst}`} aria-labelledby="attention-title">
    <h2 id="attention-title" class="sr-only">Needs attention</h2>
    <ul>
      {#each shown as item (item.id)}
        <li>
          <a href={item.href} class={`item ${item.tone}`}>
            <span class="dot" aria-hidden="true"></span>
            <span class="text">
              <span class="title">{item.title}</span>
              <span class="detail">{item.detail}</span>
            </span>
            {#if item.since}<span class="since"
                ><RelativeTime value={item.since} /></span
              >{/if}
            <ChevronRight class="chev" aria-hidden="true" />
          </a>
        </li>
      {/each}
    </ul>
    {#if overflow > 0}
      <a class="more" href="/alerts">+{overflow} more</a>
    {/if}
  </section>
{:else}
  <p class="clear" role="status">
    <CircleCheck aria-hidden="true" />
    All resources healthy, collectors fresh, no open incidents.
  </p>
{/if}

<style>
  .strip {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-5);
    padding: var(--space-2);
    border: 1px solid var(--strip-border);
    border-radius: var(--radius);
    background: var(--strip-bg);
  }
  .strip.critical {
    --strip-border: var(--critical-border);
    --strip-bg: var(--critical-bg);
  }
  .strip.warn {
    --strip-border: var(--warn-border);
    --strip-bg: var(--warn-bg);
  }
  ul {
    display: flex;
    flex: 1;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .item {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-height: 40px;
    padding: var(--space-1) var(--space-3) var(--space-1) var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text);
    text-decoration: none;
  }
  .item:hover {
    border-color: var(--border-strong);
    text-decoration: none;
  }
  .dot {
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--dot);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--dot) 25%, transparent);
  }
  .critical .dot {
    --dot: var(--critical-solid);
  }
  .warn .dot {
    --dot: var(--warn-solid);
  }
  .neutral .dot {
    --dot: var(--neutral-solid);
    box-shadow: none;
  }
  .text {
    display: grid;
    line-height: 1.25;
  }
  .title {
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .detail {
    color: var(--text-2);
    font-size: var(--text-xs);
  }
  .since {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .item :global(.chev) {
    width: 14px;
    height: 14px;
    color: var(--text-3);
  }
  .more {
    padding: 0 var(--space-3);
    color: var(--text-2);
    font-size: var(--text-sm);
    font-weight: 500;
  }
  .clear {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-5);
    color: var(--ok-fg);
    font-size: var(--text-sm);
  }
  .clear :global(svg) {
    width: 16px;
    height: 16px;
  }
</style>
