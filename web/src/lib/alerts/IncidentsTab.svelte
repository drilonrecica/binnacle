<script lang="ts">
  import Eye from '@lucide/svelte/icons/eye';
  import History from '@lucide/svelte/icons/history';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import type { Incident } from '../api/incidents';
  import { incidentTitle, targetLabel } from '../api/incidents';
  import { incidentPath, resourcePath } from '../router';
  import DataTable, { type Column } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Status from '../ui/Status.svelte';
  import { formatClock, formatSpan } from '../ui/relative-time';
  import { severityLabel, severityTone } from '../ui/status';
  import { seen } from './seen.svelte';

  let {
    incidents,
    mode,
    names,
    loading = false,
    limit,
  }: {
    incidents: Incident[];
    /** Open incidents need attention; resolved ones are history. */
    mode: 'open' | 'resolved';
    names: Map<string, string>;
    loading?: boolean;
    /** The page size asked for, to say when older rows are not shown. */
    limit?: number;
  } = $props();

  function targetName(incident: Incident) {
    if (incident.targetType === 'resource')
      return names.get(incident.targetId) ?? incident.targetId;
    return targetLabel(incident.targetType, incident.targetId);
  }

  function title(incident: Incident) {
    // Stored titles embed the raw target id; show the resource name (UI-18).
    return incidentTitle(incident, targetName(incident));
  }

  const columns = $derived<Column<Incident>[]>([
    {
      key: 'severity',
      label: mode === 'open' ? 'Severity' : 'Status',
      width: '120px',
      sortable: mode === 'open',
      sortValue: (row) => (row.severity === 'critical' ? 0 : 1),
      cell: severityCell,
    },
    {
      key: 'title',
      label: 'Incident',
      sortable: true,
      sortValue: (row) => title(row),
      cell: titleCell,
    },
    { key: 'target', label: 'Target', cell: targetCell },
    {
      key: 'alerts',
      label: mode === 'open' ? 'Firing / alerts' : 'Alerts',
      align: 'right',
      width: '120px',
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
      label: mode === 'open' ? 'Open for' : 'Lasted',
      align: 'right',
      width: '110px',
      hideBelow: 'lg',
      cell: durationCell,
    },
  ]);
</script>

{#snippet severityCell(row: Incident)}
  {#if row.status === 'open'}
    <Status
      tone={severityTone(row.severity)}
      label={severityLabel(row.severity)}
    />
  {:else}
    <Status tone="ok" label="Resolved" />
  {/if}
{/snippet}
{#snippet seenNote(row: Incident)}
  {@const mark = row.status === 'open' ? seen.get(row.id) : undefined}
  {#if mark}
    <span class="seen"
      ><Eye aria-hidden="true" />Seen by {mark.by} at {formatClock(
        mark.at,
        undefined,
        false,
      )}<span class="sr-only"> in this browser</span></span
    >
  {/if}
{/snippet}
{#snippet titleCell(row: Incident)}
  <div class="title-cell">
    <a class="title" href={incidentPath(row.id)}>{title(row)}</a>
    {@render seenNote(row)}
  </div>
{/snippet}
{#snippet targetCell(row: Incident)}
  {#if row.targetType === 'resource'}
    <a class="mono" href={resourcePath(row.targetId)}>{targetName(row)}</a>
  {:else}
    <span class="mono">{targetName(row)}</span>
  {/if}
{/snippet}
{#snippet alertsCell(row: Incident)}
  {#if mode === 'open'}
    <span class="num"
      >{row.firingAlertCount}<span class="muted">
        / {row.alertCount}</span
      ></span
    >
  {:else}
    <span class="num">{row.alertCount}</span>
  {/if}
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
      {@render severityCell(row)}
    </div>
    <div class="card-meta">
      <span class="mono">{targetName(row)}</span>
      {#if mode === 'open'}
        <span>{row.firingAlertCount} of {row.alertCount} firing</span>
      {:else}
        <span
          >{row.alertCount}
          {row.alertCount === 1 ? 'alert' : 'alerts'}</span
        >
      {/if}
      <span
        >opened <RelativeTime value={row.openedAt} /> · {formatSpan(
          row.openedAt,
          row.resolvedAt ?? Date.now(),
        )}</span
      >
    </div>
    {@render seenNote(row)}
  </a>
{/snippet}
{#snippet empty()}
  {#if mode === 'open'}
    <EmptyState
      title="No open incidents"
      description="Firing alerts group into incidents by target. Nothing needs attention right now."
      tone="ok"
    >
      {#snippet icon()}<ShieldCheck />{/snippet}
    </EmptyState>
  {:else}
    <EmptyState
      title="No resolved incidents"
      description="Incidents land here when their last alert clears. Resolved incidents are kept for a year."
    >
      {#snippet icon()}<History />{/snippet}
    </EmptyState>
  {/if}
{/snippet}

<div class="toolbar">
  <p class="hint">
    {#if mode === 'open'}
      Incidents open with the first firing alert and resolve themselves when
      every alert clears. Mark one as seen on its page to note, in this browser,
      that someone is on it.
    {:else}
      The most recent resolved incidents, newest first.
    {/if}
  </p>
</div>
<DataTable
  rows={incidents}
  {columns}
  rowKey={(row) => row.id}
  caption={mode === 'open' ? 'Open incidents' : 'Resolved incidents'}
  rowHref={(row) => incidentPath(row.id)}
  {loading}
  {empty}
  {mobileCard}
/>
{#if limit && incidents.length >= limit}
  <p class="more">
    Showing the {limit} most recent. Older incidents are kept but not listed here.
  </p>
{/if}

<style>
  .toolbar {
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--line);
  }
  .hint,
  .more {
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .more {
    padding: var(--space-3) var(--space-4);
    border-top: 1px solid var(--line);
  }
  .title-cell {
    display: grid;
    gap: 2px;
    white-space: normal;
  }
  .title {
    font-weight: var(--weight-semibold);
  }
  .seen {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .seen :global(svg) {
    width: 13px;
    height: 13px;
  }
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }
  .muted {
    color: var(--muted);
  }
  .card {
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
    gap: var(--space-1) var(--space-3);
    color: var(--muted);
    font-size: var(--text-meta);
  }
</style>
