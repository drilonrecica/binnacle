<script lang="ts">
  import { onMount } from 'svelte';
  import Search from '@lucide/svelte/icons/search';
  import type { LiveStore } from '../live.svelte';
  import { prefs } from '../preferences.svelte';
  import { router } from '../router.svelte';
  import { listIncidents, type Incident } from '../api/incidents';
  import { fetchSparklines, type SparklineResponse } from '../api/metrics';
  import { shell } from '../shell/shell-state.svelte';
  import { prioritizedResources, staleResource } from '../watch';
  import { attentionItems } from '../overview/attention';
  import AttentionStrip from '../overview/AttentionStrip.svelte';
  import HeadroomRow from '../overview/HeadroomRow.svelte';
  import ActivityFeed from '../overview/ActivityFeed.svelte';
  import SetupChecklist from '../overview/SetupChecklist.svelte';
  import ResourceTable from '../resources/ResourceTable.svelte';
  import Card from '../ui/Card.svelte';
  import Input from '../ui/Input.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import SegmentedControl from '../ui/SegmentedControl.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import type { SortState } from '../ui/DataTable.svelte';

  let { live }: { live: LiveStore } = $props();

  let incidents = $state<Incident[]>([]);
  let sparklines = $state<SparklineResponse | null>(null);
  let search = $state('');
  let filter = $state<'all' | 'attention'>(
    router.param('filter') === 'attention' ? 'attention' : 'all',
  );
  let sort = $state<SortState | undefined>(undefined);

  const snapshot = $derived(live.snapshot);
  const attention = $derived(attentionItems(snapshot, incidents));

  const ordered = $derived(
    snapshot
      ? prioritizedResources(
          snapshot.resources.filter(
            (resource) => resource.status !== 'archived',
          ),
          prefs.value.pinnedResources,
        )
      : [],
  );
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
        resource.id,
      ]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(needle)),
    );
  });

  async function loadIncidents() {
    if (document.visibilityState !== 'visible') return;
    try {
      incidents = await listIncidents({ status: 'open', limit: 100 });
      shell.reportOpenIncidents(incidents.length);
    } catch {
      /* The attention strip degrades to live-snapshot signals only. */
    }
  }

  async function loadSparklines() {
    if (document.visibilityState !== 'visible') return;
    try {
      sparklines = await fetchSparklines(['cpu', 'memory'], '1h');
    } catch {
      /* Row sparklines fall back to the live ring buffer. */
    }
  }

  onMount(() => {
    void loadIncidents();
    void loadSparklines();
    const incidentTimer = window.setInterval(
      () => void loadIncidents(),
      30_000,
    );
    const sparklineTimer = window.setInterval(
      () => void loadSparklines(),
      60_000,
    );
    return () => {
      window.clearInterval(incidentTimer);
      window.clearInterval(sparklineTimer);
    };
  });
</script>

<PageHeader
  title="Overview"
  description="Host headroom, resource health, and what changed on this server."
/>

{#if !snapshot}
  <div
    class="loading"
    role="status"
    aria-label="Waiting for the first live sample"
  >
    <Skeleton height={120} />
    <Skeleton height={320} />
  </div>
{:else}
  <AttentionStrip items={attention} />
  <HeadroomRow {snapshot} />

  <div class="columns">
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
        {sparklines}
        bind:sort
        emptyTitle={search || filter === 'attention'
          ? 'No matching resources'
          : 'No resources yet'}
        emptyDescription={filter === 'attention' && !search
          ? 'Every resource is healthy and reporting.'
          : 'Try a different filter or clear the search.'}
      />
    </Card>
    <div class="side">
      <SetupChecklist />
      <ActivityFeed {live} />
    </div>
  </div>
{/if}

<style>
  .loading {
    display: grid;
    gap: var(--space-5);
  }
  .columns {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 340px;
    gap: var(--space-5);
    align-items: start;
  }
  .side {
    display: grid;
    gap: var(--space-5);
  }
  .toolbar {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .search {
    width: 220px;
  }
  @media (max-width: 1180px) {
    .columns {
      grid-template-columns: 1fr;
    }
  }
  @media (max-width: 720px) {
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
