<script lang="ts">
  import { missing } from '../i18n';
  import Boxes from '@lucide/svelte/icons/boxes';
  import Pin from '@lucide/svelte/icons/pin';
  import type { LiveResource } from '../api/resources';
  import { categoryLabel, resourceGroup } from '../api/resources';
  import type { SparklineResponse } from '../api/metrics';
  import { liveSeries } from '../live-series.svelte';
  import { mergeSparkline } from '../series';
  import { resourcePath } from '../router';
  import { formatBytes, formatNumber, formatRate } from '../i18n';
  import { resourceStatusLabel } from '../resource-sort';
  import { staleResource } from '../watch';
  import DataTable, {
    type Column,
    type SortState,
  } from '../ui/DataTable.svelte';
  import Badge from '../ui/Badge.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import Sparkline from '../ui/Sparkline.svelte';
  import StatusPill from '../ui/StatusPill.svelte';
  import { normalizeStatus, statusTone } from '../ui/status';

  let {
    resources,
    snapshotTs,
    sparklines = null,
    grouped = true,
    sort = $bindable(),
    emptyTitle = 'No resources yet',
    emptyDescription = 'Containers appear here as soon as the Docker collector sees them.',
    density = 'default',
    groupKey,
    groupSequence,
    pinned = [],
    layout = 'auto',
  }: {
    resources: LiveResource[];
    snapshotTs: string;
    sparklines?: SparklineResponse | null;
    grouped?: boolean;
    sort?: SortState;
    emptyTitle?: string;
    emptyDescription?: string;
    density?: 'default' | 'compact';
    /** Replaces the project / environment grouping, e.g. to add "Pinned". */
    groupKey?: (resource: LiveResource) => string;
    /** Replaces the alphabetical group order. */
    groupSequence?: string[];
    /** Ids of pinned resources; their rows carry a pin. */
    pinned?: string[];
    /** `cards` shows the card form at every width. */
    layout?: 'auto' | 'table' | 'cards';
  } = $props();

  const pinnedIds = $derived(new Set(pinned));

  const hasGroups = $derived(
    grouped &&
      resources.some(
        (resource) =>
          resource.project || resource.environment || resource.infrastructure,
      ),
  );

  function isStale(resource: LiveResource) {
    return (
      resource.status !== 'archived' && staleResource(resource, snapshotTs)
    );
  }

  function displayStatus(resource: LiveResource) {
    return isStale(resource) ? 'stale' : resourceStatusLabel(resource);
  }

  function spark(resource: LiveResource, metric: 'cpu' | 'memory') {
    const base = sparklines?.resources[resource.id]?.[metric];
    const live = liveSeries.resources[resource.id]?.[metric] ?? [];
    if (base && sparklines) {
      return mergeSparkline(
        base,
        sparklines.stepSeconds,
        new Date(sparklines.to).getTime(),
        live,
      );
    }
    return liveSeries.resourceSparkline(resource.id, metric, 3_600_000);
  }

  const groupOrder = $derived.by(() => {
    if (groupSequence) return groupSequence;
    const keys = [...new Set(resources.map(resourceGroup))]
      .filter((key) => key !== 'Infrastructure' && key !== 'Ungrouped')
      .sort();
    return [...keys, 'Ungrouped', 'Infrastructure'];
  });

  const columns: Column<LiveResource>[] = [
    {
      key: 'status',
      label: 'Status',
      width: '132px',
      sortable: true,
      sortValue: (row) => displayStatus(row),
      cell: statusCell,
    },
    {
      key: 'name',
      label: 'Resource',
      sortable: true,
      sortValue: (row) => row.name,
      cell: nameCell,
    },
    {
      key: 'cpu',
      label: 'CPU',
      align: 'right',
      width: '150px',
      sortable: true,
      sortValue: (row) => (isStale(row) ? null : row.cpuHostPct),
      cell: cpuCell,
    },
    {
      key: 'memory',
      label: 'Memory',
      align: 'right',
      width: '160px',
      sortable: true,
      sortValue: (row) => (isStale(row) ? null : row.memoryBytes),
      cell: memoryCell,
    },
    {
      key: 'network',
      label: 'Network',
      align: 'right',
      width: '140px',
      hideBelow: 'lg',
      sortable: true,
      sortValue: (row) => row.rxBps,
      cell: networkCell,
    },
    {
      key: 'components',
      label: 'Containers',
      align: 'right',
      width: '110px',
      hideBelow: 'lg',
      sortable: true,
      sortValue: (row) => row.components?.length ?? 0,
      cell: componentsCell,
    },
  ];
</script>

{#snippet statusCell(row: LiveResource)}
  <StatusPill status={displayStatus(row)} />
{/snippet}

{#snippet nameCell(row: LiveResource)}
  <div class="name-cell">
    <a class="name" href={resourcePath(row.id)}
      >{#if pinnedIds.has(row.id)}<Pin
          class="pin"
          aria-label="Pinned"
          role="img"
        />{/if}{row.name}</a
    >
    <span class="context">
      <Badge tone="neutral">{categoryLabel(row.category)}</Badge>
      {#if row.context && !row.project}<span class="context-text"
          >{row.context}</span
        >{/if}
    </span>
  </div>
{/snippet}

{#snippet cpuCell(row: LiveResource)}
  {@const stale = isStale(row)}
  <div class="metric-cell">
    <Sparkline
      values={stale ? [] : spark(row, 'cpu')}
      width={60}
      height={22}
      tone={stale ? 'neutral' : 'accent'}
    />
    <span class="num"
      >{stale || row.cpuHostPct == null
        ? missing
        : `${formatNumber(row.cpuHostPct)}%`}</span
    >
  </div>
{/snippet}

{#snippet memoryCell(row: LiveResource)}
  {@const stale = isStale(row)}
  <div class="metric-cell">
    <Sparkline
      values={stale ? [] : spark(row, 'memory')}
      width={60}
      height={22}
      tone={stale ? 'neutral' : 'info'}
    />
    <span class="num"
      >{stale || row.memoryBytes == null
        ? missing
        : formatBytes(row.memoryBytes)}</span
    >
  </div>
{/snippet}

{#snippet networkCell(row: LiveResource)}
  <span class="net num">
    <span>↓ {row.rxBps == null ? missing : formatRate(row.rxBps)}</span>
    <span>↑ {row.txBps == null ? missing : formatRate(row.txBps)}</span>
  </span>
{/snippet}

{#snippet componentsCell(row: LiveResource)}
  {@const components = row.components ?? []}
  <span class="components">
    <span class="dots" aria-hidden="true">
      {#each components.slice(0, 6) as component (component.id)}
        <span
          class={`cdot ${statusTone(normalizeStatus(component.healthStatus === 'starting' ? 'starting' : component.status))}`}
        ></span>
      {/each}
    </span>
    <span class="num">{components.length}</span>
  </span>
{/snippet}

{#snippet groupLabel({ key, count }: { key: string; count: number })}
  <span class="group-name">{key}</span>
  <span class="group-count">{count}</span>
{/snippet}

{#snippet empty()}
  <EmptyState title={emptyTitle} description={emptyDescription}>
    {#snippet icon()}<Boxes />{/snippet}
  </EmptyState>
{/snippet}

{#snippet mobileCard(row: LiveResource)}
  {@const stale = isStale(row)}
  <a class="card-link" href={resourcePath(row.id)}>
    <div class="card-top">
      <span class="name"
        >{#if pinnedIds.has(row.id)}<Pin
            class="pin"
            aria-label="Pinned"
            role="img"
          />{/if}{row.name}</span
      >
      <StatusPill status={displayStatus(row)} size="sm" />
    </div>
    <div class="card-meta">
      <Badge tone="neutral">{categoryLabel(row.category)}</Badge>
      <span class="num"
        >{stale || row.cpuHostPct == null
          ? missing
          : `${formatNumber(row.cpuHostPct)}% CPU`}</span
      >
      <span class="num"
        >{stale || row.memoryBytes == null
          ? missing
          : formatBytes(row.memoryBytes)}</span
      >
      <span class="num">{row.components?.length ?? 0} containers</span>
    </div>
  </a>
{/snippet}

<DataTable
  rows={resources}
  {columns}
  rowKey={(row) => row.id}
  caption="Resources"
  bind:sort
  rowHref={(row) => resourcePath(row.id)}
  groupBy={groupKey ?? (hasGroups ? resourceGroup : undefined)}
  {groupOrder}
  {groupLabel}
  {empty}
  {mobileCard}
  {density}
  {layout}
/>

<style>
  .name-cell {
    display: grid;
    gap: 2px;
    min-width: 160px;
  }
  .name {
    font-weight: 600;
    color: var(--fg);
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .name :global(.pin) {
    width: 12px;
    height: 12px;
    margin-right: var(--space-1);
    color: var(--accent);
    vertical-align: -1px;
  }
  .context {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--muted);
    font-size: var(--text-mono);
  }
  .context-text {
    font-family: var(--font-mono);
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .metric-cell {
    display: inline-flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-3);
  }
  .metric-cell .num {
    min-width: 5.5ch;
    text-align: right;
  }
  .net {
    display: inline-grid;
    gap: 1px;
    color: var(--muted);
    font-size: var(--text-mono);
    line-height: 1.2;
    text-align: right;
  }
  .components {
    display: inline-flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2);
  }
  .dots {
    display: inline-flex;
    gap: 3px;
  }
  .cdot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--neutral-solid);
  }
  .cdot.ok {
    background: var(--ok-solid);
  }
  .cdot.warn {
    background: var(--warn-solid);
  }
  .cdot.critical {
    background: var(--critical-solid);
  }
  .group-name {
    text-transform: none;
    font-size: var(--text-meta);
    letter-spacing: 0;
  }
  .group-count {
    padding: 0 6px;
    border-radius: var(--radius-control);
    background: var(--line);
    color: var(--muted);
    font-family: var(--font-mono);
    font-weight: 500;
  }
  .card-link {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    color: var(--fg);
    text-decoration: none;
  }
  .card-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }
  .card-meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3);
    color: var(--muted);
    font-size: var(--text-mono);
  }
</style>
