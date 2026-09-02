<script lang="ts">
  import { onMount } from 'svelte';
  import Activity from '@lucide/svelte/icons/activity';
  import type { LiveStore } from '../live.svelte';
  import { listEvents, type HistoricalEvent } from '../api/events';
  import { errorMessage } from '../api/client';
  import { resourcePath } from '../router';
  import Card from '../ui/Card.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import { severityTone } from '../ui/status';

  let { live, limit = 10 }: { live: LiveStore; limit?: number } = $props();

  interface Row {
    key: string;
    at: string;
    severity: string;
    message: string;
    resourceId?: string;
    resourceName?: string;
    live: boolean;
  }

  let history = $state<HistoricalEvent[]>([]);
  let loading = $state(true);
  let error = $state('');

  const names = $derived(
    new Map(
      (live.snapshot?.resources ?? []).map((resource) => [
        resource.id,
        resource.name,
      ]),
    ),
  );

  const rows = $derived.by<Row[]>(() => {
    const fromLive: Row[] = live.events
      .map((event) => ({
        key: `live:${event.id}`,
        at: event.at ?? new Date().toISOString(),
        severity: event.severity ?? 'info',
        message: event.message,
        resourceId: event.resourceId,
        resourceName: event.resourceId
          ? names.get(event.resourceId)
          : undefined,
        live: true,
      }))
      .reverse();
    // Persisted rows duplicate live ones once the writer catches up.
    const seen = new Set(fromLive.map((row) => `${row.at}|${row.message}`));
    const fromHistory: Row[] = history
      .filter((event) => !seen.has(`${event.ts}|${event.summary}`))
      .map((event) => ({
        key: `hist:${event.id}`,
        at: event.ts,
        severity: event.severity,
        message: event.summary,
        resourceId: event.resourceId,
        resourceName: event.resourceId
          ? names.get(event.resourceId)
          : undefined,
        live: false,
      }));
    return [...fromLive, ...fromHistory].slice(0, limit);
  });

  onMount(() => {
    const controller = new AbortController();
    const to = new Date();
    listEvents(
      { from: new Date(to.getTime() - 24 * 3_600_000), to },
      controller.signal,
    )
      .then((events) => (history = events))
      .catch((reason) => {
        if (!controller.signal.aborted) error = errorMessage(reason);
      })
      .finally(() => (loading = false));
    return () => controller.abort();
  });
</script>

<Card title="Activity" id="activity-title" padded={false}>
  {#snippet actions()}<a class="all" href="/activity">View all</a>{/snippet}
  {#if loading}
    <div class="loading" role="status" aria-label="Loading activity">
      <Skeleton lines={4} height={14} />
    </div>
  {:else if error && !rows.length}
    <p class="error" role="alert">{error}</p>
  {:else if !rows.length}
    <EmptyState
      title="Nothing happened yet"
      description="Container starts, stops, deployments, and alerts show up here."
      compact
    >
      {#snippet icon()}<Activity />{/snippet}
    </EmptyState>
  {:else}
    <ol class="feed">
      {#each rows as row (row.key)}
        <li class:live={row.live}>
          <span class={`sev ${severityTone(row.severity)}`} aria-hidden="true"
          ></span>
          <div class="body">
            <p class="message">{row.message}</p>
            <p class="meta">
              {#if row.resourceId}
                <a href={resourcePath(row.resourceId)}
                  >{row.resourceName ?? row.resourceId}</a
                >
                <span aria-hidden="true">·</span>
              {/if}
              <RelativeTime value={row.at} />
              <span class="sr-only">, severity {row.severity}</span>
            </p>
          </div>
        </li>
      {/each}
    </ol>
  {/if}
</Card>

<style>
  .all {
    color: var(--accent-text);
    font-size: var(--text-sm);
    font-weight: 500;
  }
  .loading {
    padding: var(--space-4);
  }
  .error {
    padding: var(--space-4);
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  .feed {
    margin: 0;
    padding: var(--space-2) 0;
    list-style: none;
  }
  li {
    display: flex;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-4);
  }
  li.live {
    animation: flash 1.2s var(--ease-out);
  }
  @keyframes flash {
    from {
      background: var(--accent-bg);
    }
  }
  .sev {
    flex: none;
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
    gap: 2px;
    min-width: 0;
  }
  .message {
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .meta a {
    color: var(--text-2);
    font-family: var(--font-mono);
  }
</style>
