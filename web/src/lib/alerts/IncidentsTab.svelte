<script lang="ts">
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import type { Incident } from '../api/incidents';
  import { targetLabel } from '../api/incidents';
  import { incidentPath, resourcePath } from '../router';
  import Badge from '../ui/Badge.svelte';
  import DataTable, { type Column } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import SegmentedControl from '../ui/SegmentedControl.svelte';
  import StatusPill from '../ui/StatusPill.svelte';
  import { formatSpan } from '../ui/relative-time';
  import { severityLabel, severityTone } from '../ui/status';

  let {
    incidents,
    loading = false,
    names,
  }: {
    incidents: Incident[];
    loading?: boolean;
    names: Map<string, string>;
  } = $props();

  let filter = $state<'open' | 'resolved' | 'all'>('open');

  const rows = $derived(
    incidents.filter((incident) =>
      filter === 'all'
        ? true
        : filter === 'open'
          ? incident.status === 'open'
          : incident.status !== 'open',
    ),
  );

  function targetName(incident: Incident) {
    if (incident.targetType === 'resource')
      return names.get(incident.targetId) ?? incident.targetId;
    return targetLabel(incident.targetType, incident.targetId);
  }

  function title(incident: Incident) {
    // Backend titles for resource incidents embed the raw id; show the name instead.
    if (
      incident.targetType === 'resource' &&
      incident.title.includes(incident.targetId)
    ) {
      return `Incident on ${targetName(incident)}`;
    }
    return incident.title;
  }

  const columns: Column<Incident>[] = [
    { key: 'status', label: 'Status', width: '130px', cell: statusCell },
    {
      key: 'title',
      label: 'Incident',
      sortable: true,
      sortValue: (row) => title(row),
      cell: titleCell,
    },
    { key: 'target', label: 'Target', hideBelow: 900, cell: targetCell },
    {
      key: 'alerts',
      label: 'Alerts',
      align: 'right',
      width: '110px',
      hideBelow: 720,
      cell: alertsCell,
    },
    {
      key: 'opened',
      label: 'Opened',
      align: 'right',
      width: '120px',
      sortable: true,
      sortValue: (row) => Date.parse(row.openedAt),
      cell: openedCell,
    },
    {
      key: 'duration',
      label: 'Duration',
      align: 'right',
      width: '110px',
      hideBelow: 1100,
      cell: durationCell,
    },
  ];
</script>

{#snippet statusCell(row: Incident)}
  {#if row.status === 'open'}
    <StatusPill
      status={row.severity === 'critical' ? 'down' : 'degraded'}
      label={severityLabel(row.severity)}
    />
  {:else}
    <StatusPill status="healthy" label="Resolved" />
  {/if}
{/snippet}
{#snippet titleCell(row: Incident)}
  <a class="title" href={incidentPath(row.id)}>{title(row)}</a>
{/snippet}
{#snippet targetCell(row: Incident)}
  {#if row.targetType === 'resource'}
    <a class="mono" href={resourcePath(row.targetId)}>{targetName(row)}</a>
  {:else}
    <span class="mono">{targetName(row)}</span>
  {/if}
{/snippet}
{#snippet alertsCell(row: Incident)}
  <span class="num"
    >{row.firingAlertCount}<span class="muted"> / {row.alertCount}</span></span
  >
{/snippet}
{#snippet openedCell(row: Incident)}
  <RelativeTime value={row.openedAt} />
{/snippet}
{#snippet durationCell(row: Incident)}
  <span class="num"
    >{formatSpan(row.openedAt, row.resolvedAt ?? Date.now())}</span
  >
{/snippet}
{#snippet mobileCard(row: Incident)}
  <a class="card" href={incidentPath(row.id)}>
    <div class="card-top">
      <span class="title">{title(row)}</span>
      {#if row.status === 'open'}
        <Badge tone={severityTone(row.severity)} dot
          >{severityLabel(row.severity)}</Badge
        >
      {:else}
        <Badge tone="ok" dot>Resolved</Badge>
      {/if}
    </div>
    <div class="card-meta">
      <span class="mono">{targetName(row)}</span>
      <span>{row.firingAlertCount} firing</span>
      <RelativeTime value={row.openedAt} />
    </div>
  </a>
{/snippet}
{#snippet empty()}
  <EmptyState
    title={filter === 'open' ? 'No open incidents' : 'No incidents'}
    description={filter === 'open'
      ? 'Firing alerts group into incidents by target. Nothing needs attention right now.'
      : 'Resolved incidents are kept for a year.'}
    tone={filter === 'open' ? 'ok' : 'neutral'}
  >
    {#snippet icon()}<ShieldCheck />{/snippet}
  </EmptyState>
{/snippet}

<div class="toolbar">
  <SegmentedControl
    label="Incident status"
    size="sm"
    bind:value={filter}
    options={[
      {
        value: 'open',
        label: `Open ${incidents.filter((incident) => incident.status === 'open').length}`,
      },
      { value: 'resolved', label: 'Resolved' },
      { value: 'all', label: 'All' },
    ]}
  />
  <p class="hint">
    Incidents open with the first firing alert and resolve themselves when every
    alert clears. There is nothing to acknowledge or assign.
  </p>
</div>
<DataTable
  {rows}
  {columns}
  rowKey={(row) => row.id}
  caption="Incidents"
  rowHref={(row) => incidentPath(row.id)}
  {loading}
  {empty}
  {mobileCard}
/>

<style>
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3) var(--space-4);
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border);
  }
  .hint {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .title {
    font-weight: 600;
  }
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .muted {
    color: var(--text-3);
  }
  .card {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    color: var(--text);
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
    gap: var(--space-3);
    color: var(--text-3);
    font-size: var(--text-xs);
  }
</style>
