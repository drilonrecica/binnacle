<script lang="ts">
  import Activity from '@lucide/svelte/icons/activity';
  import ScrollText from '@lucide/svelte/icons/scroll-text';
  import { eventTypeLabel, type HistoricalEvent } from '../api/events';
  import { logsPath, resourcePath } from '../router';
  import Badge from '../ui/Badge.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import { formatClock } from '../ui/relative-time';
  import { severityTone } from '../ui/status';
  import { tooltip } from '../ui/tooltip';

  let {
    events,
    loading = false,
    error = '',
    resourceNames = new Map<string, string>(),
    showResource = true,
    emptyTitle = 'No events in this range',
    emptyDescription = 'Container lifecycle changes, deployments, collector state, and alerts appear here.',
    highlightIds = new Set<string>(),
  }: {
    events: HistoricalEvent[];
    loading?: boolean;
    error?: string;
    resourceNames?: Map<string, string>;
    showResource?: boolean;
    emptyTitle?: string;
    emptyDescription?: string;
    /** Ids of rows that arrived live and should flash briefly. */
    highlightIds?: Set<string>;
  } = $props();

  function dayKey(ts: string) {
    return new Date(ts).toDateString();
  }

  const days = $derived.by(() => {
    const out: Array<{
      key: string;
      label: string;
      events: HistoricalEvent[];
    }> = [];
    for (const event of events) {
      const key = dayKey(event.ts);
      const last = out[out.length - 1];
      if (last && last.key === key) last.events.push(event);
      else
        out.push({
          key,
          label: new Intl.DateTimeFormat(undefined, {
            dateStyle: 'medium',
          }).format(new Date(event.ts)),
          events: [event],
        });
    }
    return out;
  });
</script>

{#if loading && !events.length}
  <div class="loading" role="status" aria-label="Loading events">
    <Skeleton lines={6} height={16} />
  </div>
{:else if error && !events.length}
  <p class="error" role="alert">{error}</p>
{:else if !events.length}
  <EmptyState title={emptyTitle} description={emptyDescription}>
    {#snippet icon()}<Activity />{/snippet}
  </EmptyState>
{:else}
  <ol class="days">
    {#each days as day (day.key)}
      <li>
        <h3 class="day">{day.label}</h3>
        <ol class="events">
          {#each day.events as event (event.id)}
            <li class="event" class:live={highlightIds.has(event.id)}>
              <span
                class="time num"
                use:tooltip={new Date(event.ts).toLocaleString()}
                >{formatClock(event.ts)}</span
              >
              <span
                class={`sev ${severityTone(event.severity)}`}
                aria-hidden="true"
              ></span>
              <span class="sr-only">{event.severity}</span>
              <div class="body">
                <p class="summary">{event.summary}</p>
                <p class="meta">
                  <Badge
                    tone={severityTone(event.severity) === 'neutral'
                      ? 'neutral'
                      : severityTone(event.severity)}
                    >{eventTypeLabel(event.type)}</Badge
                  >
                  {#if showResource && event.resourceId}
                    <a href={resourcePath(event.resourceId)}
                      >{resourceNames.get(event.resourceId) ??
                        event.resourceId}</a
                    >
                  {/if}
                  {#if event.containerInstanceId}
                    <code
                      class="container"
                      use:tooltip={event.containerInstanceId}
                      >{event.containerInstanceId.slice(0, 12)}</code
                    >
                  {/if}
                  <span class="source">{event.source}</span>
                  <span class="ago"><RelativeTime value={event.ts} /></span>
                </p>
              </div>
              {#if event.resourceId}
                <a
                  class="logs"
                  href={logsPath(event.resourceId, event.ts)}
                  use:tooltip={'Open logs around this moment'}
                  aria-label="Open logs around this event"
                >
                  <ScrollText aria-hidden="true" />
                </a>
              {/if}
            </li>
          {/each}
        </ol>
      </li>
    {/each}
  </ol>
{/if}

<style>
  .loading {
    padding: var(--space-4);
  }
  .error {
    padding: var(--space-4);
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  .days,
  .events {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .day {
    position: sticky;
    top: 0;
    z-index: 1;
    padding: var(--space-2) var(--space-4);
    background: var(--bg-subtle);
    color: var(--text-3);
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }
  .event {
    display: grid;
    grid-template-columns: 92px 8px minmax(0, 1fr) auto;
    gap: var(--space-3);
    align-items: start;
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border);
  }
  .event.live {
    animation: flash 1.4s var(--ease-out);
  }
  @keyframes flash {
    from {
      background: var(--accent-bg);
    }
  }
  .time {
    padding-top: 2px;
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .sev {
    width: 8px;
    height: 8px;
    margin-top: 6px;
    border-radius: 50%;
    background: var(--neutral-solid);
  }
  .sev.warn {
    background: var(--warn-solid);
  }
  .sev.critical {
    background: var(--critical-solid);
  }
  .sev.info {
    background: var(--info-solid);
  }
  .body {
    display: grid;
    gap: var(--space-1);
    min-width: 0;
  }
  .summary {
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .meta a {
    color: var(--text-2);
    font-weight: 500;
  }
  .container {
    padding: 1px 5px;
    border-radius: 4px;
    background: var(--surface-2);
    font-size: 11px;
  }
  .source {
    text-transform: capitalize;
  }
  .logs {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: var(--radius-sm);
    color: var(--text-3);
  }
  .logs:hover {
    background: var(--surface-2);
    color: var(--text);
  }
  .logs :global(svg) {
    width: 15px;
    height: 15px;
  }
  @media (max-width: 600px) {
    .event {
      grid-template-columns: 8px minmax(0, 1fr) auto;
    }
    .time {
      display: none;
    }
  }
</style>
