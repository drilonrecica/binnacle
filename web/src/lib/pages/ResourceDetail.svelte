<script lang="ts">
  import { onMount } from 'svelte';
  import BellOff from '@lucide/svelte/icons/bell-off';
  import Copy from '@lucide/svelte/icons/copy';
  import ScrollText from '@lucide/svelte/icons/scroll-text';
  import type { LiveStore } from '../live.svelte';
  import { liveSeries } from '../live-series.svelte';
  import {
    categoryLabel,
    getResource,
    resourceGroup,
    type ArchivedResource,
    type LiveComponent,
    type LiveResource,
  } from '../api/resources';
  import { listEvents, type HistoricalEvent } from '../api/events';
  import { errorMessage, isApiError } from '../api/client';
  import { logsPath } from '../router';
  import { formatBytes, formatNumber, formatRate } from '../i18n';
  import { resourceStatusLabel } from '../resource-sort';
  import { staleResource } from '../watch';
  import type { ChartGroup } from '../charts/MetricCharts.svelte';
  import MetricCharts from '../charts/MetricCharts.svelte';
  import SilenceDialog from '../alerts/SilenceDialog.svelte';
  import EventList from '../activity/EventList.svelte';
  import ResourceAlerts from '../resources/ResourceAlerts.svelte';
  import HistoryDeletionCard from '../settings/HistoryDeletionCard.svelte';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import DataTable, { type Column } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import IconButton from '../ui/IconButton.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import Stat from '../ui/Stat.svelte';
  import StatusPill from '../ui/StatusPill.svelte';
  import Tabs, { tabPanelId, tabId } from '../ui/Tabs.svelte';
  import { toasts } from '../ui/toast.svelte';

  let { live, id }: { live: LiveStore; id: string } = $props();

  let fetched = $state<LiveResource | ArchivedResource | null>(null);
  let notFound = $state(false);
  let error = $state('');
  let tab = $state('metrics');
  let silenceOpen = $state(false);
  let alertsRefresh = $state(0);
  let events = $state<HistoricalEvent[]>([]);
  let eventsLoading = $state(false);
  let eventsError = $state('');

  const current = $derived(
    live.snapshot?.resources.find((resource) => resource.id === id) ?? null,
  );
  const resource = $derived<LiveResource | ArchivedResource | null>(
    current ?? fetched,
  );
  const isArchived = $derived(
    Boolean(
      resource &&
      (resource.status === 'archived' ||
        (!current && 'archivedAt' in resource)),
    ),
  );
  const stale = $derived(
    current && live.snapshot ? staleResource(current, live.snapshot.ts) : false,
  );
  const statusValue = $derived(
    isArchived
      ? 'archived'
      : stale
        ? 'stale'
        : current
          ? resourceStatusLabel(current)
          : (resource?.status ?? 'unknown'),
  );
  const components = $derived<LiveComponent[]>(current?.components ?? []);
  const names = $derived(
    new Map(
      (live.snapshot?.resources ?? []).map((item) => [item.id, item.name]),
    ),
  );

  function spark(metric: 'cpu' | 'memory') {
    return liveSeries
      .resourceWindow(id, metric, 3_600_000)
      .slice(-60)
      .map((sample) => sample.value);
  }

  const chartGroups: ChartGroup[] = [
    {
      key: 'cpu',
      title: 'CPU',
      description: 'Host-normalized percentage across all containers',
      metrics: [{ metric: 'cpu', label: 'CPU' }],
      format: 'percent',
    },
    {
      key: 'memory',
      title: 'Memory',
      description: 'Working set across all containers',
      metrics: [{ metric: 'memory', label: 'Memory', color: 5 }],
      format: 'bytes',
    },
    {
      key: 'network',
      title: 'Network',
      metrics: [
        { metric: 'network_rx', label: 'Receive', color: 1 },
        { metric: 'network_tx', label: 'Transmit', color: 4 },
      ],
      format: 'rate',
    },
    {
      key: 'block',
      title: 'Block I/O',
      metrics: [
        { metric: 'block_read', label: 'Read', color: 2 },
        { metric: 'block_write', label: 'Write', color: 3 },
      ],
      format: 'rate',
    },
  ];

  const componentColumns: Column<LiveComponent>[] = [
    { key: 'status', label: 'Status', width: '130px', cell: componentStatus },
    {
      key: 'name',
      label: 'Container',
      sortable: true,
      sortValue: (row) => row.name,
      cell: componentName,
    },
    {
      key: 'runtime',
      label: 'Runtime',
      hideBelow: 900,
      cell: componentRuntime,
    },
    {
      key: 'cpu',
      label: 'CPU',
      align: 'right',
      sortable: true,
      sortValue: (row) => row.cpuHostPct,
      cell: componentCpu,
    },
    {
      key: 'memory',
      label: 'Memory',
      align: 'right',
      sortable: true,
      sortValue: (row) => row.memoryBytes,
      cell: componentMemory,
    },
    {
      key: 'pids',
      label: 'PIDs',
      align: 'right',
      hideBelow: 1100,
      sortable: true,
      sortValue: (row) => row.pids,
      cell: componentPids,
    },
    {
      key: 'actions',
      label: 'Actions',
      srOnly: true,
      align: 'right',
      width: '48px',
      cell: componentActions,
    },
  ];

  async function copy(text: string, what: string) {
    try {
      await navigator.clipboard.writeText(text);
      toasts.success(`${what} copied`);
    } catch {
      toasts.error(`Could not copy the ${what.toLowerCase()}`);
    }
  }

  async function loadEvents() {
    eventsLoading = true;
    eventsError = '';
    try {
      const to = new Date();
      events = await listEvents({
        from: new Date(to.getTime() - 7 * 86_400_000),
        to,
        resourceId: id,
      });
    } catch (reason) {
      eventsError = errorMessage(reason);
    } finally {
      eventsLoading = false;
    }
  }

  $effect(() => {
    if (tab === 'activity' && !events.length && !eventsLoading && !eventsError)
      void loadEvents();
  });

  onMount(() => {
    if (current) return;
    void getResource(id)
      .then((value) => (fetched = value))
      .catch((reason) => {
        if (isApiError(reason) && reason.status === 404) notFound = true;
        else error = errorMessage(reason);
      });
  });

  const tabs = $derived(
    isArchived
      ? [
          { id: 'metrics', label: 'Metrics' },
          { id: 'activity', label: 'Activity' },
        ]
      : [
          { id: 'metrics', label: 'Metrics' },
          { id: 'containers', label: 'Containers', count: components.length },
          { id: 'alerts', label: 'Alerts' },
          { id: 'activity', label: 'Activity' },
        ],
  );
</script>

{#snippet componentStatus(row: LiveComponent)}
  <StatusPill
    status={row.healthStatus === 'starting' ? 'starting' : row.status}
  />
{/snippet}
{#snippet componentName(row: LiveComponent)}
  <div class="component-name">
    <span class="mono">{row.name}</span>
    <code class="component-id" title={row.id}>{row.id.slice(0, 12)}</code>
  </div>
{/snippet}
{#snippet componentRuntime(row: LiveComponent)}
  <span class="runtime">
    {row.runtimeState || '—'}{#if row.healthStatus}<span class="health">
        · {row.healthStatus}</span
      >{/if}
  </span>
{/snippet}
{#snippet componentCpu(row: LiveComponent)}
  {row.cpuHostPct == null ? '—' : `${formatNumber(row.cpuHostPct)}%`}
{/snippet}
{#snippet componentMemory(row: LiveComponent)}
  {row.memoryBytes == null ? '—' : formatBytes(row.memoryBytes)}
{/snippet}
{#snippet componentPids(row: LiveComponent)}
  {row.pids ?? '—'}
{/snippet}
{#snippet componentActions(row: LiveComponent)}
  <IconButton
    label="Copy container id"
    size="sm"
    onclick={() => copy(row.id, 'Container id')}><Copy /></IconButton
  >
{/snippet}

{#if resource}
  <PageHeader
    title={resource.name}
    crumbs={[{ label: 'Resources', href: '/resources' }]}
    headingId="resource-title"
  >
    {#snippet meta()}
      <StatusPill status={statusValue} />
      <Badge tone="neutral">{categoryLabel(resource.category)}</Badge>
      {#if resource.project || resource.environment || ('infrastructure' in resource && resource.infrastructure)}
        <Badge tone="accent">{resourceGroup(resource)}</Badge>
      {/if}
      {#if resource.context && !resource.project}<span class="context mono"
          >{resource.context}</span
        >{/if}
      {#if isArchived && 'archivedAt' in resource && resource.archivedAt}
        <span class="archived-at"
          >archived {new Date(resource.archivedAt).toLocaleString()}</span
        >
      {/if}
      <button
        type="button"
        class="id"
        onclick={() => copy(id, 'Resource id')}
        title="Copy resource id"
      >
        <code>{id}</code><Copy aria-hidden="true" />
      </button>
    {/snippet}
    {#snippet actions()}
      {#if !isArchived}
        <Button href={logsPath(id)}>
          {#snippet icon()}<ScrollText />{/snippet}
          Open logs
        </Button>
        <Button onclick={() => (silenceOpen = true)}>
          {#snippet icon()}<BellOff />{/snippet}
          Silence…
        </Button>
      {/if}
    {/snippet}
    {#if current}
      <div class="stats">
        <Stat
          label="CPU"
          value={stale || current.cpuHostPct == null
            ? '—'
            : formatNumber(current.cpuHostPct)}
          unit={stale || current.cpuHostPct == null ? '' : '%'}
          sparkline={spark('cpu')}
          sparklineLabel="CPU over the last hour"
          secondary="host-normalized"
        />
        <Stat
          label="Memory"
          value={stale || current.memoryBytes == null
            ? '—'
            : formatBytes(current.memoryBytes)}
          sparkline={spark('memory')}
          sparklineLabel="Memory over the last hour"
          tone="info"
          secondary="working set"
        />
        <Stat
          label="Network"
          value={stale || current.rxBps == null
            ? '—'
            : formatRate(current.rxBps)}
          secondary={current.txBps == null
            ? ''
            : `↓ receive · ↑ ${formatRate(current.txBps)}`}
          tone="accent"
        />
        <Stat
          label="Block I/O"
          value={stale || current.blockReadBps == null
            ? '—'
            : formatRate(current.blockReadBps)}
          secondary={current.blockWriteBps == null
            ? ''
            : `read · write ${formatRate(current.blockWriteBps)}`}
          tone="accent"
        />
      </div>
    {/if}
    <Tabs
      prefix="resource"
      label="Resource sections"
      param="tab"
      bind:active={tab}
      {tabs}
    />
  </PageHeader>

  <div
    id={tabPanelId('resource', 'metrics')}
    role="tabpanel"
    aria-labelledby={tabId('resource', 'metrics')}
    hidden={tab !== 'metrics'}
  >
    {#if tab === 'metrics'}
      <MetricCharts scope="resource" {id} groups={chartGroups} />
    {/if}
  </div>

  {#if !isArchived}
    <div
      id={tabPanelId('resource', 'containers')}
      role="tabpanel"
      aria-labelledby={tabId('resource', 'containers')}
      hidden={tab !== 'containers'}
    >
      {#if tab === 'containers'}
        <Card padded={false}>
          <DataTable
            rows={components}
            columns={componentColumns}
            rowKey={(row) => row.id}
            caption="Containers"
          >
            {#snippet empty()}
              <EmptyState
                title="No container detail"
                description="Container-level data appears when the Docker collector is healthy."
              />
            {/snippet}
          </DataTable>
        </Card>
      {/if}
    </div>
    <div
      id={tabPanelId('resource', 'alerts')}
      role="tabpanel"
      aria-labelledby={tabId('resource', 'alerts')}
      hidden={tab !== 'alerts'}
    >
      {#if tab === 'alerts'}
        <ResourceAlerts
          resourceId={id}
          project={resource.project}
          onsilence={() => (silenceOpen = true)}
          refreshKey={alertsRefresh}
        />
      {/if}
    </div>
  {/if}

  <div
    id={tabPanelId('resource', 'activity')}
    role="tabpanel"
    aria-labelledby={tabId('resource', 'activity')}
    hidden={tab !== 'activity'}
  >
    {#if tab === 'activity'}
      <Card padded={false}>
        <EventList
          {events}
          loading={eventsLoading}
          error={eventsError}
          resourceNames={names}
          showResource={false}
          emptyTitle="No events in the last 7 days"
        />
      </Card>
    {/if}
  </div>

  {#if isArchived}
    <div class="danger">
      <HistoryDeletionCard archivedResourceId={id} />
    </div>
  {/if}

  <SilenceDialog
    bind:open={silenceOpen}
    snapshot={live.snapshot}
    scope="resource"
    scopeId={id}
    oncreated={() => (alertsRefresh += 1)}
  />
{:else if notFound}
  <PageHeader
    title="Resource not found"
    crumbs={[{ label: 'Resources', href: '/resources' }]}
  />
  <EmptyState
    title="This resource is not being monitored"
    description="It may have been removed, renamed, or never existed. Archived resources keep their history for a while."
  >
    <Button href="/resources" variant="primary">Back to resources</Button>
  </EmptyState>
{:else if error}
  <PageHeader
    title="Resource"
    crumbs={[{ label: 'Resources', href: '/resources' }]}
  />
  <p class="error" role="alert">{error}</p>
{:else}
  <div class="loading" role="status" aria-label="Loading resource">
    <Skeleton height={32} width={280} />
    <Skeleton height={110} />
    <Skeleton height={320} />
  </div>
{/if}

<style>
  .stats {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    overflow: hidden;
  }
  .stats > :global(.stat) {
    border-right: 1px solid var(--border);
  }
  .stats > :global(.stat:last-child) {
    border-right: 0;
  }
  .context,
  .archived-at {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .mono {
    font-family: var(--font-mono);
  }
  .id {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 2px 6px;
    border: 0;
    border-radius: 4px;
    background: none;
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .id:hover {
    background: var(--surface-2);
    color: var(--text);
  }
  .id :global(svg) {
    width: 12px;
    height: 12px;
  }
  .component-name {
    display: grid;
    gap: 2px;
  }
  .component-id {
    color: var(--text-3);
    font-size: 11px;
  }
  .runtime {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .health {
    color: var(--text-3);
  }
  .danger {
    margin-top: var(--space-5);
  }
  .error {
    color: var(--critical-fg);
  }
  .loading {
    display: grid;
    gap: var(--space-4);
  }
  @media (max-width: 1000px) {
    .stats {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .stats > :global(.stat:nth-child(2)) {
      border-right: 0;
    }
    .stats > :global(.stat:nth-child(-n + 2)) {
      border-bottom: 1px solid var(--border);
    }
  }
</style>
