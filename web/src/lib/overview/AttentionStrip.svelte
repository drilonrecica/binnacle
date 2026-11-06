<script lang="ts">
  import type { AttentionItem, AttentionState } from './attention';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import StatusMarker from '../ui/StatusMarker.svelte';
  import { formatAbsolute, formatClock } from '../ui/relative-time';

  let { state, limit = 6 }: { state: AttentionState; limit?: number } =
    $props();

  const items = $derived<AttentionItem[]>(
    state.kind === 'items' || state.kind === 'unknown' ? state.items : [],
  );
  // The unknown chip takes one of the six places.
  const room = $derived(state.kind === 'unknown' ? limit - 1 : limit);
  const shown = $derived(items.slice(0, room));
  const overflow = $derived(items.length - shown.length);
</script>

<section class="attention" aria-labelledby="attention-title">
  <h2 id="attention-title" class="sr-only">Needs attention</h2>
  {#if state.kind === 'loading'}
    <div
      class="loading"
      role="status"
      aria-label="Checking what needs attention"
    >
      <Skeleton height={40} />
    </div>
  {:else if state.kind === 'clear'}
    <p class="clear" data-testid="attention-clear">
      <StatusMarker tone="ok" size={9} />
      Nothing needs attention
    </p>
  {:else}
    <ul>
      {#if state.kind === 'unknown'}
        <li>
          <span
            class="chip neutral unknown"
            data-testid="attention-unknown"
            title={formatAbsolute(state.since)}
          >
            <StatusMarker tone="neutral" size={9} />
            <span class="text">
              <span class="title">
                Status unknown since <time datetime={state.since}
                  >{formatClock(state.since, undefined, false)}</time
                >
              </span>
              <span class="detail">
                {state.reason === 'incidents'
                  ? 'Incidents could not be loaded'
                  : 'Live updates paused'}{#if state.lastUpdate}, last update
                  <RelativeTime value={state.lastUpdate} />{/if}
              </span>
            </span>
          </span>
        </li>
      {/if}
      {#each shown as item (item.id)}
        <li>
          <a href={item.href} class={`chip ${item.tone}`}>
            <StatusMarker tone={item.tone} size={9} />
            <span class="text">
              <span class="title">{item.title}</span>
              <span class="detail">{item.detail}</span>
            </span>
            {#if item.since}<span class="since"
                ><RelativeTime value={item.since} /></span
              >{/if}
          </a>
        </li>
      {/each}
      {#if overflow > 0}
        <li>
          <a class="chip more" href="/incidents">+{overflow} more</a>
        </li>
      {/if}
    </ul>
  {/if}
</section>

<style>
  .attention {
    margin-bottom: var(--space-4);
  }
  .loading {
    max-width: 320px;
  }
  ul {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .chip {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 40px;
    padding: var(--space-1) var(--space-3);
    border: 1px solid var(--chip-border, var(--line));
    border-radius: var(--radius-control);
    background: var(--chip-bg, var(--panel));
    color: var(--fg);
    text-decoration: none;
  }
  a.chip:hover {
    border-color: var(--line-strong);
    text-decoration: none;
  }
  .chip.critical {
    --chip-border: var(--critical-border);
    --chip-bg: var(--critical-bg);
  }
  .chip.warn {
    --chip-border: var(--warn-border);
    --chip-bg: var(--warn-bg);
  }
  .chip.unknown {
    border-style: dashed;
  }
  .text {
    display: grid;
    line-height: 1.25;
  }
  .title {
    font-size: var(--text-meta);
    font-weight: var(--weight-semibold);
  }
  .detail,
  .since {
    color: var(--muted);
    font-size: var(--text-mono);
  }
  .since {
    margin-left: var(--space-1);
    font-family: var(--font-mono);
  }
  .more {
    color: var(--muted);
    font-size: var(--text-meta);
    font-weight: var(--weight-medium);
  }
  .clear {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--ok-fg);
    font-size: var(--text-meta);
    font-weight: var(--weight-medium);
  }
</style>
