<script lang="ts">
  import { onMount } from 'svelte';
  import Search from '@lucide/svelte/icons/search';
  import type { LiveStore } from '../live.svelte';
  import { liveSeries } from '../live-series.svelte';
  import { prefs } from '../preferences.svelte';
  import { router } from '../router.svelte';
  import { listIncidents, type Incident } from '../api/incidents';
  import { listRules, type Rule } from '../api/alerts';
  import { shell } from '../shell/shell-state.svelte';
  import { connectionView } from '../shell/connection';
  import { staleResource } from '../watch';
  import { attentionState } from '../overview/attention';
  import {
    gaugeThresholds,
    headroomInstruments,
    headroomResources,
  } from '../overview/headroom';
  import AttentionStrip from '../overview/AttentionStrip.svelte';
  import Instruments from '../overview/Instruments.svelte';
  import ChangesFeed from '../overview/ChangesFeed.svelte';
  import SetupChecklist from '../overview/SetupChecklist.svelte';
  import ResourceTable from '../resources/ResourceTable.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import ErrorState from '../ui/ErrorState.svelte';
  import Input from '../ui/Input.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import SegmentedControl from '../ui/SegmentedControl.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import StaleBanner from '../ui/StaleBanner.svelte';
  import StatusMarker from '../ui/StatusMarker.svelte';
  import type { SortState } from '../ui/DataTable.svelte';

  let { live }: { live: LiveStore } = $props();

  const hourMs = 3_600_000;
  const openedAt = new Date().toISOString();

  let incidents = $state<Incident[] | null>(null);
  let incidentsLoadedAt = $state<string | null>(null);
  let incidentsFailed = $state(false);
  let rules = $state<Rule[]>([]);
  let search = $state('');
  let filter = $state<'all' | 'attention'>(
    router.param('filter') === 'attention' ? 'attention' : 'all',
  );
  let sort = $state<SortState | undefined>(undefined);
  // Decides when quiet data turns stale; nothing it drives is announced.
  let now = $state(Date.now());

  const snapshot = $derived(live.snapshot);
  const connection = $derived(
    connectionView(live.state, live.lastReceivedAt, now),
  );
  const streamLive = $derived(connection.tone === 'ok');
  const thresholds = $derived(gaugeThresholds(rules));
  const attention = $derived(
    attentionState({
      snapshot,
      connection: live.state,
      lastReceivedAt: live.lastReceivedAt,
      incidents,
      incidentsLoadedAt,
      incidentsFailed,
      openedAt,
      thresholds,
      now,
    }),
  );
  const instruments = $derived(
    snapshot
      ? headroomInstruments(
          snapshot,
          thresholds,
          liveSeries.hostWindow('cpu', hourMs, Date.parse(snapshot.ts)),
        )
      : null,
  );

  const grouped = $derived(
    snapshot
      ? headroomResources(
          snapshot.resources,
          prefs.value.pinnedResources,
          snapshot.ts,
        )
      : null,
  );
  const ordered = $derived(grouped?.rows ?? []);
  const needsAttention = $derived(
    snapshot
      ? ordered.filter(
          (resource) =>
            resource.status !== 'healthy' ||
            staleResource(resource, snapshot.ts),
        )
      : [],
  );
  const filtered = $derived.by(() => {
    const base = filter === 'attention' ? needsAttention : ordered;
    const needle = search.trim().toLowerCase();
    if (!needle) return base;
    return base.filter((resource) =>
      [
        resource.name,
        resource.project,
        resource.environment,
        resource.context,
        resource.category,
      ]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(needle)),
    );
  });
  // The first live snapshot never arrived and the store gave up opening.
  const failedToStart = $derived(
    !snapshot &&
      (live.state === 'disconnected' || live.state === 'unauthorized'),
  );

  async function loadIncidents() {
    if (document.visibilityState !== 'visible') return;
    try {
      const open = await listIncidents({ status: 'open', limit: 100 });
      incidents = open;
      incidentsLoadedAt = new Date().toISOString();
      incidentsFailed = false;
      shell.reportOpenIncidents(open.length);
    } catch {
      // The strip says status is unknown instead of showing an all-clear (UI-1).
      incidentsFailed = true;
    }
  }

  async function loadRules() {
    try {
      rules = await listRules();
    } catch {
      // Gauges fall back to the design thresholds.
      rules = [];
    }
  }

  onMount(() => {
    void loadIncidents();
    void loadRules();
    const incidentTimer = window.setInterval(
      () => void loadIncidents(),
      30_000,
    );
    const clock = window.setInterval(() => (now = Date.now()), 2_000);
    return () => {
      window.clearInterval(incidentTimer);
      window.clearInterval(clock);
    };
  });
</script>

<PageHeader title="Headroom">
  {#snippet meta()}
    {#if snapshot}
      <span class="num"
        >{ordered.length}
        {ordered.length === 1 ? 'resource' : 'resources'}</span
      >
      <span aria-hidden="true">·</span>
    {/if}
    <span class="connection" data-tone={connection.tone}>
      <StatusMarker tone={connection.tone} size={8} />
      {connection.label.toLowerCase()}{#if connection.since}&nbsp;{connection.since}{/if}
    </span>
  {/snippet}
</PageHeader>

{#if failedToStart}
  <ErrorState
    title="Live data could not be loaded"
    description={live.error ||
      'The live stream did not start. A proxy in front of Binnacle may be buffering it.'}
    onretry={() => live.retry()}
    retryLabel="Retry now"
  >
    <a href="/settings/system">Check Settings › System</a>
  </ErrorState>
{:else if !snapshot || !instruments || !grouped}
  <div
    class="loading"
    role="status"
    aria-label="Waiting for the first live sample"
  >
    <Skeleton height={40} />
    <Skeleton height={180} />
    <Skeleton height={320} />
  </div>
{:else}
  {#if !streamLive}
    <div class="stale">
      <StaleBanner
        message="Showing the last known values"
        since={live.lastReceivedAt}
        tone={live.state === 'disconnected' ? 'critical' : 'warn'}
      >
        {#if live.state === 'disconnected'}
          <Button size="sm" onclick={() => live.retry()}>Retry now</Button>
        {/if}
      </StaleBanner>
    </div>
  {/if}

  <AttentionStrip state={attention} />
  <Instruments set={instruments} stale={!streamLive} />

  <div class="columns">
    <div class="main">
      <Card title="Resources" id="resources-title" padded={false}>
        {#snippet actions()}
          <div class="toolbar">
            <div class="search">
              <Input
                type="search"
                placeholder="Filter resources"
                bind:value={search}
                aria-label="Filter resources"
              >
                {#snippet leading()}<Search />{/snippet}
              </Input>
            </div>
            <SegmentedControl
              label="Resource filter"
              size="sm"
              bind:value={filter}
              options={[
                { value: 'all', label: `All ${ordered.length}` },
                {
                  value: 'attention',
                  label: `Attention ${needsAttention.length}`,
                },
              ]}
            />
          </div>
        {/snippet}
        <ResourceTable
          resources={filtered}
          snapshotTs={snapshot.ts}
          summary
          bind:sort
          groupKey={grouped.groupOf}
          groupSequence={grouped.groupOrder}
          pinned={prefs.value.pinnedResources}
          emptyTitle={search || filter === 'attention'
            ? 'No matching resources'
            : 'No resources yet'}
          emptyDescription={filter === 'attention' && !search
            ? 'Every resource is healthy and reporting.'
            : search || filter === 'attention'
              ? 'Try a different filter or clear the search.'
              : 'Containers appear here as soon as the Docker collector sees them.'}
        />
      </Card>
      <SetupChecklist />
    </div>
    <ChangesFeed {live} />
  </div>
{/if}

<style>
  .loading {
    display: grid;
    gap: var(--space-4);
  }
  .stale {
    margin-bottom: var(--space-4);
  }
  .connection {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }
  .connection[data-tone='warn'] {
    color: var(--warn-fg);
  }
  .connection[data-tone='critical'] {
    color: var(--critical-fg);
  }
  .columns {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 340px;
    gap: var(--space-4);
    align-items: start;
  }
  .main {
    display: grid;
    gap: var(--space-4);
    min-width: 0;
  }
  .toolbar {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .search {
    width: 220px;
  }
  @media (max-width: 1200px) {
    .columns {
      grid-template-columns: 1fr;
    }
  }
  @media (max-width: 560px) {
    .toolbar {
      flex: 1 1 auto;
      flex-wrap: wrap;
      min-width: 0;
    }
    .search {
      flex: 1 1 100%;
      width: auto;
    }
  }
</style>
