<script lang="ts">
  import BellOff from '@lucide/svelte/icons/bell-off';
  import EllipsisVertical from '@lucide/svelte/icons/ellipsis-vertical';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import Siren from '@lucide/svelte/icons/siren';
  import type { Alert } from '../api/incidents';
  import { targetLabel } from '../api/incidents';
  import { createSilence, type SilenceScope } from '../api/silences';
  import { errorMessage } from '../api/client';
  import { incidentPath, resourcePath } from '../router';
  import Badge from '../ui/Badge.svelte';
  import DataTable, { type Column } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import IconButton from '../ui/IconButton.svelte';
  import Menu from '../ui/Menu.svelte';
  import MenuItem from '../ui/MenuItem.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import { toasts } from '../ui/toast.svelte';
  import { severityLabel, severityTone } from '../ui/status';
  import { ruleInfo } from './rule-catalog';

  let {
    alerts,
    loading = false,
    names,
    onsilence,
    onchanged,
  }: {
    alerts: Alert[];
    loading?: boolean;
    names: Map<string, string>;
    /** Open the silence dialog with a scope prefilled. */
    onsilence: (scope: SilenceScope, scopeId: string) => void;
    onchanged?: () => void;
  } = $props();

  function scopeFor(alert: Alert): { scope: SilenceScope; id: string } {
    if (alert.targetType === 'resource')
      return { scope: 'resource', id: alert.targetId };
    return { scope: 'server', id: '' };
  }

  async function quickSilence(alert: Alert, preset: '1h' | '4h') {
    const { scope, id } = scopeFor(alert);
    try {
      await createSilence({
        scopeType: scope,
        scopeId: id || undefined,
        reason: `Silenced from ${alert.message}`,
        preset,
      });
      toasts.success(`Silenced for ${preset === '1h' ? '1 hour' : '4 hours'}`, {
        description: 'Notifications pause; the alert keeps evaluating.',
      });
      onchanged?.();
    } catch (reason) {
      toasts.error('Silence failed', { description: errorMessage(reason) });
    }
  }

  const columns: Column<Alert>[] = [
    {
      key: 'severity',
      label: 'Severity',
      width: '110px',
      sortable: true,
      sortValue: (row) => (row.severity === 'critical' ? 0 : 1),
      cell: severityCell,
    },
    {
      key: 'message',
      label: 'Alert',
      sortable: true,
      sortValue: (row) => row.message,
      cell: messageCell,
    },
    { key: 'target', label: 'Target', hideBelow: 900, cell: targetCell },
    {
      key: 'observed',
      label: 'Observed',
      align: 'right',
      width: '110px',
      hideBelow: 1100,
      cell: observedCell,
    },
    {
      key: 'since',
      label: 'Since',
      align: 'right',
      width: '110px',
      sortable: true,
      sortValue: (row) => Date.parse(row.startedAt),
      cell: sinceCell,
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

{#snippet severityCell(row: Alert)}
  <Badge tone={severityTone(row.severity)} dot
    >{severityLabel(row.severity)}</Badge
  >
{/snippet}
{#snippet messageCell(row: Alert)}
  <div class="message">
    <span>{row.message}</span>
    <span class="family">{ruleInfo(row.family, row.family).title}</span>
  </div>
{/snippet}
{#snippet targetCell(row: Alert)}
  {#if row.targetType === 'resource'}
    <a class="mono" href={resourcePath(row.targetId)}
      >{names.get(row.targetId) ?? row.targetId}</a
    >
  {:else}
    <span class="mono">{targetLabel(row.targetType, row.targetId)}</span>
  {/if}
{/snippet}
{#snippet observedCell(row: Alert)}
  <span class="num"
    >{row.observedValue == null
      ? '—'
      : Math.round(row.observedValue * 10) / 10}</span
  >
{/snippet}
{#snippet sinceCell(row: Alert)}
  <RelativeTime value={row.startedAt} />
{/snippet}
{#snippet actionsCell(row: Alert)}
  <Menu label={`Actions for ${row.message}`}>
    {#snippet trigger(props)}
      <IconButton label="Alert actions" size="sm" {...props}
        ><EllipsisVertical /></IconButton
      >
    {/snippet}
    <MenuItem onselect={() => quickSilence(row, '1h')}
      >{#snippet icon()}<BellOff />{/snippet}Silence 1 hour</MenuItem
    >
    <MenuItem onselect={() => quickSilence(row, '4h')}
      >{#snippet icon()}<BellOff />{/snippet}Silence 4 hours</MenuItem
    >
    <MenuItem onselect={() => onsilence(scopeFor(row).scope, scopeFor(row).id)}
      >{#snippet icon()}<BellOff />{/snippet}Silence…</MenuItem
    >
    {#if row.incidentId}
      <MenuItem href={incidentPath(row.incidentId)}
        >{#snippet icon()}<Siren />{/snippet}Open incident</MenuItem
      >
    {/if}
  </Menu>
{/snippet}
{#snippet empty()}
  <EmptyState
    title="No alerts firing"
    description="Every evaluated rule is within its threshold."
    tone="ok"
  >
    {#snippet icon()}<ShieldCheck />{/snippet}
  </EmptyState>
{/snippet}

<DataTable
  rows={alerts}
  {columns}
  rowKey={(row) => row.id}
  caption="Firing alerts"
  {loading}
  {empty}
/>

<style>
  .message {
    display: grid;
    gap: 2px;
    white-space: normal;
  }
  .family {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
</style>
