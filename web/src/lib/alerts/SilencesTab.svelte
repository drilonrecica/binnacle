<script lang="ts">
  import BellOff from '@lucide/svelte/icons/bell-off';
  import Plus from '@lucide/svelte/icons/plus';
  import { deleteSilence, silenceActive, type Silence } from '../api/silences';
  import { errorMessage } from '../api/client';
  import { resourcePath } from '../router';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import ConfirmDialog from '../ui/ConfirmDialog.svelte';
  import DataTable, { type Column } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import { toasts } from '../ui/toast.svelte';
  import { formatAbsolute } from '../ui/relative-time';

  let {
    silences,
    loading = false,
    names,
    rules,
    oncreate,
    onchanged,
  }: {
    silences: Silence[];
    loading?: boolean;
    names: Map<string, string>;
    rules: Map<string, string>;
    oncreate: () => void;
    onchanged?: () => void;
  } = $props();

  let ending = $state<Silence | null>(null);
  let confirmOpen = $state(false);

  const active = $derived(
    silences
      .filter((silence) => silenceActive(silence))
      .sort((a, b) => Date.parse(a.endsAt) - Date.parse(b.endsAt)),
  );
  const expired = $derived(
    silences
      .filter((silence) => !silenceActive(silence))
      .sort((a, b) => Date.parse(b.endsAt) - Date.parse(a.endsAt))
      .slice(0, 25),
  );

  function scopeText(silence: Silence) {
    switch (silence.scopeType) {
      case 'server':
        return 'Whole server';
      case 'project':
        return `Project ${silence.scopeId}`;
      case 'resource':
        return names.get(silence.scopeId ?? '') ?? silence.scopeId ?? '';
      case 'rule':
        return rules.get(silence.scopeId ?? '') ?? silence.scopeId ?? '';
    }
  }

  async function end() {
    if (!ending) return;
    try {
      await deleteSilence(ending.id);
      toasts.success('Silence ended', {
        description: 'Notifications resume for this scope.',
      });
      ending = null;
      onchanged?.();
    } catch (reason) {
      toasts.error('Silence could not be ended', {
        description: errorMessage(reason),
      });
      throw reason;
    }
  }

  const columns: Column<Silence>[] = [
    { key: 'scope', label: 'Scope', cell: scopeCell },
    { key: 'reason', label: 'Reason', cell: reasonCell },
    {
      key: 'ends',
      label: 'Ends',
      align: 'right',
      width: '150px',
      cell: endsCell,
    },
    {
      key: 'actions',
      label: 'Actions',
      srOnly: true,
      align: 'right',
      width: '110px',
      cell: actionsCell,
    },
  ];
  const expiredColumns: Column<Silence>[] = [
    { key: 'scope', label: 'Scope', cell: scopeCell },
    { key: 'reason', label: 'Reason', cell: reasonCell },
    {
      key: 'ended',
      label: 'Ended',
      align: 'right',
      width: '150px',
      cell: endedCell,
    },
  ];
</script>

{#snippet scopeCell(row: Silence)}
  <div class="scope">
    <Badge tone={row.scopeType === 'server' ? 'warn' : 'info'}
      >{row.scopeType}</Badge
    >
    {#if row.scopeType === 'resource' && row.scopeId}
      <a class="mono" href={resourcePath(row.scopeId)}>{scopeText(row)}</a>
    {:else}
      <span>{scopeText(row)}</span>
    {/if}
  </div>
{/snippet}
{#snippet reasonCell(row: Silence)}
  <span class="reason">{row.reason}</span>
{/snippet}
{#snippet endsCell(row: Silence)}
  <span class="ends"
    ><RelativeTime value={row.endsAt} /><span class="muted"
      >{formatAbsolute(row.endsAt)}</span
    ></span
  >
{/snippet}
{#snippet endedCell(row: Silence)}
  <RelativeTime value={row.endsAt} />
{/snippet}
{#snippet actionsCell(row: Silence)}
  <Button
    size="sm"
    variant="ghost"
    onclick={() => {
      ending = row;
      confirmOpen = true;
    }}>End now</Button
  >
{/snippet}
{#snippet empty()}
  <EmptyState
    title="No active silences"
    description="Silences pause notifications for a server, project, resource, or rule while alerts keep evaluating."
  >
    {#snippet icon()}<BellOff />{/snippet}
    <Button variant="primary" onclick={oncreate}>
      {#snippet icon()}<Plus />{/snippet}
      New silence
    </Button>
  </EmptyState>
{/snippet}

{#if active.length}
  <div class="toolbar">
    <p class="hint">
      A still-failing condition notifies again the moment its silence ends.
    </p>
    <Button size="sm" variant="primary" onclick={oncreate}>
      {#snippet icon()}<Plus />{/snippet}
      New silence
    </Button>
  </div>
{/if}
<DataTable
  rows={active}
  {columns}
  rowKey={(row) => row.id}
  caption="Active silences"
  {loading}
  {empty}
/>
{#if expired.length}
  <h3 class="expired-title">Recently expired</h3>
  <DataTable
    rows={expired}
    columns={expiredColumns}
    rowKey={(row) => row.id}
    caption="Expired silences"
    density="compact"
  />
{/if}

<ConfirmDialog
  bind:open={confirmOpen}
  title="End this silence now?"
  description="Notifications for the scope resume immediately."
  confirmLabel="End silence"
  onconfirm={end}
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
  .scope {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .reason {
    white-space: normal;
  }
  .ends {
    display: inline-grid;
    gap: 1px;
    text-align: right;
  }
  .muted {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .expired-title {
    padding: var(--space-3) var(--space-4) var(--space-1);
    border-top: 1px solid var(--border);
    color: var(--text-3);
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }
</style>
