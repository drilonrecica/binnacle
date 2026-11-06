<script lang="ts">
  import { onMount } from 'svelte';
  import History from '@lucide/svelte/icons/history';
  import type { LiveStore } from '../live.svelte';
  import { listEvents, type HistoricalEvent } from '../api/events';
  import { errorMessage } from '../api/client';
  import { resourcePath } from '../router';
  import Card from '../ui/Card.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import ErrorState from '../ui/ErrorState.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import StatusMarker from '../ui/StatusMarker.svelte';
  import { severityLabel, severityTone } from '../ui/status';
  import { groupRepeats, mergeChanges, repeatText } from './changes';

  let { live, limit = 10 }: { live: LiveStore; limit?: number } = $props();

  let history = $state<HistoricalEvent[]>([]);
  let loading = $state(true);
  let error = $state('');
  let controller: AbortController | null = null;

  const names = $derived(
    new Map(
      (live.snapshot?.resources ?? []).map((resource) => [
        resource.id,
        resource.name,
      ]),
    ),
  );
  const groups = $derived(
    groupRepeats(mergeChanges(live.events, history), limit),
  );

  function load() {
    controller?.abort();
    const current = new AbortController();
    controller = current;
    loading = true;
    error = '';
    const to = new Date();
    listEvents(
      { from: new Date(to.getTime() - 24 * 3_600_000), to, limit: 200 },
      current.signal,
    )
      .then((events) => (history = events))
      .catch((reason) => {
        if (!current.signal.aborted) error = errorMessage(reason);
      })
      .finally(() => {
        if (controller === current) loading = false;
      });
  }

  onMount(() => {
    load();
    return () => controller?.abort();
  });
</script>

<Card title="Changes" id="changes-title" padded={false}>
  {#snippet actions()}<a class="all" href="/changes">All changes</a>{/snippet}
  {#if loading && !groups.length}
    <div class="loading" role="status" aria-label="Loading changes">
      <Skeleton lines={4} height={14} />
    </div>
  {:else if error && !groups.length}
    <div class="error">
      <ErrorState
        title="Changes could not be loaded"
        description={error}
        onretry={load}
        compact
      />
    </div>
  {:else if !groups.length}
    <EmptyState
      title="No changes in the last 24 hours"
      description="Container starts, stops, deployments and alerts show up here."
      compact
    >
      {#snippet icon()}<History />{/snippet}
    </EmptyState>
  {:else}
    {#if error}
      <p class="partial" role="alert">
        Older changes could not be loaded; showing live ones only.
      </p>
    {/if}
    <ol class="feed">
      {#each groups as group (group.key)}
        {@const row = group.latest}
        {@const repeats = repeatText(group)}
        <li class:live={row.live}>
          <span class="sev">
            <StatusMarker tone={severityTone(row.severity)} size={8} />
          </span>
          <div class="body">
            <p class="message">
              {row.message}{#if repeats}<span class="repeats">
                  · {repeats}</span
                >{/if}
            </p>
            <p class="meta">
              {#if row.resourceId}
                <a href={resourcePath(row.resourceId)}
                  >{names.get(row.resourceId) ?? 'Removed resource'}</a
                >
                <span aria-hidden="true">·</span>
              {/if}
              <RelativeTime value={row.at} />
              <span class="sr-only">, {severityLabel(row.severity)}</span>
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
    font-size: var(--text-meta);
    font-weight: var(--weight-medium);
  }
  .loading,
  .error {
    padding: var(--space-4);
  }
  .partial {
    padding: var(--space-2) var(--space-4) 0;
    color: var(--warn-fg);
    font-size: var(--text-meta);
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
  @media (prefers-reduced-motion: reduce) {
    li.live {
      animation: none;
    }
  }
  .sev {
    display: flex;
    flex: none;
    padding-top: 5px;
  }
  .body {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .message {
    font-size: var(--text-meta);
    overflow-wrap: anywhere;
  }
  .repeats {
    color: var(--muted);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    color: var(--muted);
    font-size: var(--text-mono);
  }
  .meta a {
    color: var(--muted);
  }
</style>
