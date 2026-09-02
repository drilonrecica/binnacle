<script lang="ts">
  import EllipsisVertical from '@lucide/svelte/icons/ellipsis-vertical';
  import HeartPulse from '@lucide/svelte/icons/heart-pulse';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Play from '@lucide/svelte/icons/play';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import type { LiveSnapshot } from '../live.svelte';
  import {
    deleteCheck,
    failureLabel,
    runCheck,
    updateCheck,
    checkToInput,
    type Check,
  } from '../api/checks';
  import { errorMessage } from '../api/client';
  import { resourcePath } from '../router';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import ConfirmDialog from '../ui/ConfirmDialog.svelte';
  import DataTable, { type Column } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import IconButton from '../ui/IconButton.svelte';
  import Menu from '../ui/Menu.svelte';
  import MenuItem from '../ui/MenuItem.svelte';
  import MenuSeparator from '../ui/MenuSeparator.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import StatusPill from '../ui/StatusPill.svelte';
  import Switch from '../ui/Switch.svelte';
  import { toasts } from '../ui/toast.svelte';
  import { tooltip } from '../ui/tooltip';
  import CheckDialog from './CheckDialog.svelte';

  let {
    checks,
    loading = false,
    snapshot,
    names,
    onchanged,
    resourceId = '',
  }: {
    checks: Check[];
    loading?: boolean;
    snapshot: LiveSnapshot | null;
    names: Map<string, string>;
    onchanged?: () => void;
    /** Preselect this resource when creating. */
    resourceId?: string;
  } = $props();

  let dialogOpen = $state(false);
  let editing = $state<Check | null>(null);
  let deleting = $state<Check | null>(null);
  let confirmOpen = $state(false);
  let busy = $state<string | null>(null);

  function openCreate() {
    editing = null;
    dialogOpen = true;
  }

  function openEdit(check: Check) {
    editing = check;
    dialogOpen = true;
  }

  async function toggle(check: Check, enabled: boolean) {
    busy = check.id;
    try {
      await updateCheck(check.id, { ...checkToInput(check), enabled });
      toasts.success(enabled ? 'Check enabled' : 'Check disabled', {
        description: check.name,
      });
      onchanged?.();
    } catch (reason) {
      toasts.error('Check could not be updated', {
        description: errorMessage(reason),
      });
    } finally {
      busy = null;
    }
  }

  async function run(check: Check) {
    busy = check.id;
    try {
      const result = await runCheck(check.id);
      if (result.status === 'success')
        toasts.success(`${check.name} passed`, {
          description:
            `${result.httpStatus ?? ''} in ${result.latencyMs ?? '?'} ms`.trim(),
        });
      else
        toasts.warning(`${check.name} failed`, {
          description: failureLabel(result.failureCode),
        });
      onchanged?.();
    } catch (reason) {
      toasts.error('Check could not run', {
        description: errorMessage(reason),
      });
    } finally {
      busy = null;
    }
  }

  async function remove() {
    if (!deleting) return;
    await deleteCheck(deleting.id);
    toasts.success('Check deleted', { description: deleting.name });
    deleting = null;
    onchanged?.();
  }

  function statusFor(check: Check) {
    if (!check.enabled) return { status: 'unknown', label: 'Disabled' };
    if (!check.state) return { status: 'unknown', label: 'Not run yet' };
    if (check.state.status === 'success')
      return { status: 'healthy', label: 'Passing' };
    if (check.state.status === 'failure')
      return { status: check.required ? 'down' : 'degraded', label: 'Failing' };
    return { status: 'unknown', label: check.state.status };
  }

  const columns: Column<Check>[] = [
    {
      key: 'status',
      label: 'Status',
      width: '130px',
      sortable: true,
      sortValue: (row) => statusFor(row).label,
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
      hideBelow: 900,
      sortable: true,
      sortValue: (row) => names.get(row.resourceId) ?? row.resourceId,
      cell: resourceCell,
    },
    { key: 'result', label: 'Last result', hideBelow: 1100, cell: resultCell },
    {
      key: 'schedule',
      label: 'Every',
      align: 'right',
      width: '90px',
      hideBelow: 720,
      sortable: true,
      sortValue: (row) => row.interval,
      cell: scheduleCell,
    },
    {
      key: 'enabled',
      label: 'Enabled',
      align: 'center',
      width: '90px',
      cell: enabledCell,
    },
    {
      key: 'actions',
      label: 'Actions',
      srOnly: true,
      align: 'right',
      width: '56px',
      cell: actionsCell,
    },
  ];
</script>

{#snippet statusCell(row: Check)}
  {@const state = statusFor(row)}
  <StatusPill status={state.status} label={state.label} />
{/snippet}
{#snippet nameCell(row: Check)}
  <div class="name">
    <span class="title">{row.name}</span>
    <span class="url mono" use:tooltip={row.url}>{row.method} {row.url}</span>
    <span class="tags"
      ><Badge tone={row.required ? 'critical' : 'neutral'}
        >{row.required ? 'Required' : 'Optional'}</Badge
      ></span
    >
  </div>
{/snippet}
{#snippet resourceCell(row: Check)}
  <a class="mono" href={resourcePath(row.resourceId)}
    >{names.get(row.resourceId) ?? row.resourceId}</a
  >
{/snippet}
{#snippet resultCell(row: Check)}
  {#if row.state}
    <div class="result">
      <span>
        {#if row.state.status === 'success'}
          HTTP {row.state.httpStatus || 'ok'} · {row.state.latencyMs ?? '?'} ms
        {:else}
          {failureLabel(row.state.failureCode)}{#if row.state.httpStatus}
            · HTTP {row.state.httpStatus}{/if}
        {/if}
      </span>
      <span class="muted"
        ><RelativeTime
          value={row.state.checkedAt}
        />{#if row.state.consecutiveFailures > 1}
          · {row.state.consecutiveFailures} failures in a row{/if}</span
      >
    </div>
  {:else}
    <span class="muted">—</span>
  {/if}
{/snippet}
{#snippet scheduleCell(row: Check)}
  <span class="num">{Math.round(row.interval / 1_000_000_000)}s</span>
{/snippet}
{#snippet enabledCell(row: Check)}
  <Switch
    label={`${row.name} enabled`}
    checked={row.enabled}
    busy={busy === row.id}
    onchange={(next) => toggle(row, next)}
  />
{/snippet}
{#snippet actionsCell(row: Check)}
  <Menu label={`Actions for ${row.name}`}>
    {#snippet trigger(props)}
      <IconButton label="Check actions" size="sm" {...props}
        ><EllipsisVertical /></IconButton
      >
    {/snippet}
    <MenuItem onselect={() => run(row)}
      >{#snippet icon()}<Play />{/snippet}Run now</MenuItem
    >
    <MenuItem onselect={() => openEdit(row)}
      >{#snippet icon()}<Pencil />{/snippet}Edit…</MenuItem
    >
    <MenuSeparator />
    <MenuItem
      danger
      onselect={() => {
        deleting = row;
        confirmOpen = true;
      }}>{#snippet icon()}<Trash2 />{/snippet}Delete…</MenuItem
    >
  </Menu>
{/snippet}
{#snippet empty()}
  <EmptyState
    title="No health checks yet"
    description="Probe an HTTP endpoint on a schedule. Required checks mark a resource down when they fail."
  >
    {#snippet icon()}<HeartPulse />{/snippet}
    <Button variant="primary" onclick={openCreate}>
      {#snippet icon()}<Plus />{/snippet}
      New check
    </Button>
  </EmptyState>
{/snippet}

{#if checks.length}
  <div class="toolbar">
    <p class="hint">
      Checks run from this server every 10 seconds to 1 hour. Loopback and
      private targets are blocked unless enabled in the deployment.
    </p>
    <Button size="sm" variant="primary" onclick={openCreate}>
      {#snippet icon()}<Plus />{/snippet}
      New check
    </Button>
  </div>
{/if}
<DataTable
  rows={checks}
  {columns}
  rowKey={(row) => row.id}
  caption="Health checks"
  {loading}
  {empty}
/>

<CheckDialog
  bind:open={dialogOpen}
  {snapshot}
  check={editing}
  {resourceId}
  onsaved={() => onchanged?.()}
/>
<ConfirmDialog
  bind:open={confirmOpen}
  title="Delete this check?"
  description={deleting
    ? `${deleting.name} will stop running. Its alert history stays.`
    : ''}
  confirmLabel="Delete check"
  onconfirm={remove}
/>

<style>
  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border);
  }
  .hint {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .name {
    display: grid;
    gap: 2px;
    white-space: normal;
  }
  .title {
    font-weight: 600;
  }
  .url {
    max-width: 380px;
    color: var(--text-3);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .tags {
    display: flex;
  }
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .result {
    display: grid;
    gap: 2px;
    font-size: var(--text-xs);
  }
  .muted {
    color: var(--text-3);
  }
</style>
