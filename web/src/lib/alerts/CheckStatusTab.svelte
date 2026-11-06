<script lang="ts" module>
  import type { Check } from '../api/checks';
  import { formatAbsolute, formatRelative } from '../ui/relative-time';

  /** Where checks are created, edited and deleted. */
  export const manageChecksHref = '/settings/alerting';

  /**
   * When an enabled check runs next: one interval after its last run. Null
   * for disabled checks and for checks that have not run yet.
   */
  export function nextRunAt(check: Check): number | null {
    if (!check.enabled || !check.state) return null;
    const last = Date.parse(check.state.checkedAt);
    if (!Number.isFinite(last)) return null;
    return last + check.interval / 1_000_000;
  }

  /**
   * "Next run" text. The shared relative formatter calls anything under 90 s
   * ahead "just now", which reads wrong for a schedule, so near runs get
   * their own words.
   */
  export function nextRunLabel(at: number | null, now = Date.now()): string {
    if (at == null) return missing;
    const ahead = at - now;
    if (ahead <= 0) return 'due now';
    if (ahead < 60_000) return 'in <1m';
    return formatRelative(at, now);
  }

  export function checkStatus(check: Check): {
    tone: 'ok' | 'warn' | 'critical' | 'neutral';
    label: string;
  } {
    if (!check.enabled) return { tone: 'neutral', label: 'Disabled' };
    if (!check.state) return { tone: 'neutral', label: 'Not run yet' };
    if (check.state.status === 'success')
      return { tone: 'ok', label: 'Passing' };
    if (check.state.status === 'failure')
      return {
        tone: check.required ? 'critical' : 'warn',
        label: 'Failing',
      };
    return { tone: 'neutral', label: check.state.status };
  }
</script>

<script lang="ts">
  import { missing } from '../i18n';
  import HeartPulse from '@lucide/svelte/icons/heart-pulse';
  import Settings2 from '@lucide/svelte/icons/settings-2';
  import { failureLabel } from '../api/checks';
  import { resourcePath } from '../router';
  import Button from '../ui/Button.svelte';
  import DataTable, { type Column } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Status from '../ui/Status.svelte';
  import { tooltip } from '../ui/tooltip';

  /**
   * Live pass/fail status of every health check. Read-only by design: the
   * checks are configuration and are managed under Settings › Alerting.
   */
  let {
    checks,
    names,
    loading = false,
  }: {
    checks: Check[];
    names: Map<string, string>;
    loading?: boolean;
  } = $props();

  const rank: Record<string, number> = {
    Failing: 0,
    'Not run yet': 1,
    Passing: 2,
    Disabled: 3,
  };

  const columns: Column<Check>[] = [
    {
      key: 'status',
      label: 'Status',
      width: '130px',
      sortable: true,
      sortValue: (row) => rank[checkStatus(row).label] ?? 1,
      cell: statusCell,
    },
    {
      key: 'name',
      label: 'Check',
      sortable: true,
      sortValue: (row) => row.name,
      cell: nameCell,
    },
    {
      key: 'resource',
      label: 'Resource',
      sortable: true,
      sortValue: (row) => names.get(row.resourceId) ?? row.resourceId,
      cell: resourceCell,
    },
    {
      key: 'http',
      label: 'HTTP',
      align: 'right',
      width: '80px',
      cell: httpCell,
    },
    {
      key: 'latency',
      label: 'Latency',
      align: 'right',
      width: '90px',
      sortable: true,
      sortValue: (row) => row.state?.latencyMs ?? -1,
      cell: latencyCell,
    },
    {
      key: 'last',
      label: 'Last run',
      align: 'right',
      width: '110px',
      cell: lastCell,
    },
    {
      key: 'next',
      label: 'Next run',
      align: 'right',
      width: '110px',
      hideBelow: 'lg',
      cell: nextCell,
    },
  ];
</script>

{#snippet statusCell(row: Check)}
  {@const state = checkStatus(row)}
  <Status tone={state.tone} label={state.label} />
{/snippet}
{#snippet nameCell(row: Check)}
  <div class="name">
    <span class="title">{row.name}</span>
    <span class="url mono" use:tooltip={row.url}>{row.method} {row.url}</span>
    {#if row.state?.status === 'failure'}
      <span class="failure"
        >{failureLabel(
          row.state.failureCode,
        )}{#if row.state.consecutiveFailures > 1}
          · {row.state.consecutiveFailures} failures in a row{/if}</span
      >
    {/if}
  </div>
{/snippet}
{#snippet resourceCell(row: Check)}
  <a class="mono" href={resourcePath(row.resourceId)}
    >{names.get(row.resourceId) ?? row.resourceId}</a
  >
{/snippet}
{#snippet httpCell(row: Check)}
  <span class="num">{row.state?.httpStatus || missing}</span>
{/snippet}
{#snippet latencyCell(row: Check)}
  <span class="num"
    >{row.state?.latencyMs == null
      ? missing
      : `${row.state.latencyMs} ms`}</span
  >
{/snippet}
{#snippet lastCell(row: Check)}
  <RelativeTime value={row.state?.checkedAt} />
{/snippet}
{#snippet nextCell(row: Check)}
  {@const at = nextRunAt(row)}
  {#if at == null}
    <span class="num muted">—</span>
  {:else}
    <time
      class="num"
      datetime={new Date(at).toISOString()}
      use:tooltip={formatAbsolute(at)}>{nextRunLabel(at)}</time
    >
  {/if}
{/snippet}
{#snippet mobileCard(row: Check)}
  {@const state = checkStatus(row)}
  <div class="card">
    <div class="card-top">
      <span class="title">{row.name}</span>
      <Status tone={state.tone} label={state.label} />
    </div>
    <div class="card-meta">
      <a class="mono" href={resourcePath(row.resourceId)}
        >{names.get(row.resourceId) ?? row.resourceId}</a
      >
      {#if row.state}
        <span
          >HTTP {row.state.httpStatus || missing} · {row.state.latencyMs ??
            missing} ms</span
        >
        <span>last <RelativeTime value={row.state.checkedAt} /></span>
      {/if}
    </div>
  </div>
{/snippet}
{#snippet empty()}
  <EmptyState
    title="No health checks yet"
    description="Health checks probe an HTTP endpoint on a schedule. Required checks mark a resource down when they fail."
  >
    {#snippet icon()}<HeartPulse />{/snippet}
    <Button href={manageChecksHref}>
      {#snippet icon()}<Settings2 />{/snippet}
      Set up checks
    </Button>
  </EmptyState>
{/snippet}

<div class="toolbar">
  <p class="hint">
    Live results from this server. Checks are created and edited under Settings
    › Alerting.
  </p>
  <Button size="sm" href={manageChecksHref}>
    {#snippet icon()}<Settings2 />{/snippet}
    Manage checks
  </Button>
</div>
<DataTable
  rows={checks}
  {columns}
  rowKey={(row) => row.id}
  caption="Health check status"
  {loading}
  {empty}
  {mobileCard}
/>

<style>
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3) var(--space-4);
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--line);
  }
  .hint {
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .name {
    display: grid;
    gap: 2px;
    white-space: normal;
  }
  .title {
    font-weight: var(--weight-semibold);
  }
  .url {
    max-width: 380px;
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .muted {
    color: var(--muted);
  }
  .failure {
    color: var(--critical-fg);
    font-size: var(--text-meta);
  }
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }
  .card {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
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
