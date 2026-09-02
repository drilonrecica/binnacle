<script lang="ts">
  import { onMount } from 'svelte';
  import BellRing from '@lucide/svelte/icons/bell-ring';
  import EllipsisVertical from '@lucide/svelte/icons/ellipsis-vertical';
  import Mail from '@lucide/svelte/icons/mail';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Plus from '@lucide/svelte/icons/plus';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import Send from '@lucide/svelte/icons/send';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Webhook from '@lucide/svelte/icons/webhook';
  import {
    channelTarget,
    deleteChannel,
    deliveryStatusLabel,
    listChannels,
    listDeliveries,
    retryDelivery,
    testChannel,
    updateChannel,
    type Channel,
  } from '../api/channels';
  import type { Delivery } from '../api/incidents';
  import { errorMessage } from '../api/client';
  import { incidentPath } from '../router';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import ConfirmDialog from '../ui/ConfirmDialog.svelte';
  import DataTable, { type Column } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import IconButton from '../ui/IconButton.svelte';
  import Menu from '../ui/Menu.svelte';
  import MenuItem from '../ui/MenuItem.svelte';
  import MenuSeparator from '../ui/MenuSeparator.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import StatusPill from '../ui/StatusPill.svelte';
  import Switch from '../ui/Switch.svelte';
  import { toasts } from '../ui/toast.svelte';
  import { tooltip } from '../ui/tooltip';
  import ChannelDialog from './ChannelDialog.svelte';

  let channels = $state<Channel[]>([]);
  let deliveries = $state<Delivery[]>([]);
  let loading = $state(true);
  let error = $state('');
  let dialogOpen = $state(false);
  let editing = $state<Channel | null>(null);
  let deleting = $state<Channel | null>(null);
  let confirmOpen = $state(false);
  let busy = $state<string | null>(null);

  const channelNames = $derived(
    new Map(channels.map((channel) => [channel.id, channel.name])),
  );

  async function load(quiet = false) {
    if (!quiet) loading = true;
    error = '';
    try {
      const [nextChannels, nextDeliveries] = await Promise.all([
        listChannels(),
        listDeliveries({ limit: 50 }),
      ]);
      channels = nextChannels;
      deliveries = nextDeliveries;
    } catch (reason) {
      error = errorMessage(reason);
    } finally {
      loading = false;
    }
  }

  async function toggle(channel: Channel, enabled: boolean) {
    busy = channel.id;
    try {
      await updateChannel(channel.id, { enabled });
      toasts.success(enabled ? 'Channel enabled' : 'Channel disabled', {
        description: channel.name,
      });
      await load(true);
    } catch (reason) {
      toasts.error('Channel could not be updated', {
        description: errorMessage(reason),
      });
    } finally {
      busy = null;
    }
  }

  async function test(channel: Channel) {
    busy = channel.id;
    try {
      await testChannel(channel.id);
      toasts.success('Test queued', {
        description: `A test notification is on its way to ${channel.name}. Watch the delivery history below.`,
      });
      window.setTimeout(() => void load(true), 2000);
    } catch (reason) {
      toasts.error('Test could not be sent', {
        description: errorMessage(reason),
      });
    } finally {
      busy = null;
    }
  }

  async function remove() {
    if (!deleting) return;
    await deleteChannel(deleting.id);
    toasts.success('Channel deleted', { description: deleting.name });
    deleting = null;
    await load(true);
  }

  async function retry(delivery: Delivery) {
    busy = delivery.id;
    try {
      await retryDelivery(delivery.id);
      toasts.success('Retry queued');
      await load(true);
    } catch (reason) {
      toasts.error('Retry failed', { description: errorMessage(reason) });
    } finally {
      busy = null;
    }
  }

  onMount(() => {
    void load();
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') void load(true);
    }, 20_000);
    return () => window.clearInterval(timer);
  });

  const deliveryColumns: Column<Delivery>[] = [
    { key: 'status', label: 'Status', width: '120px', cell: deliveryStatus },
    { key: 'event', label: 'Event', cell: deliveryEvent },
    { key: 'channel', label: 'Channel', cell: deliveryChannel },
    {
      key: 'incident',
      label: 'Incident',
      hideBelow: 900,
      cell: deliveryIncident,
    },
    {
      key: 'attempts',
      label: 'Attempts',
      align: 'right',
      width: '90px',
      hideBelow: 1100,
      cell: deliveryAttempts,
    },
    {
      key: 'when',
      label: 'Updated',
      align: 'right',
      width: '130px',
      cell: deliveryWhen,
    },
    {
      key: 'actions',
      label: 'Actions',
      srOnly: true,
      align: 'right',
      width: '100px',
      cell: deliveryActions,
    },
  ];
</script>

{#snippet deliveryStatus(row: Delivery)}
  <StatusPill
    status={row.status === 'succeeded'
      ? 'healthy'
      : row.status === 'permanent_failure'
        ? 'down'
        : row.status === 'cancelled'
          ? 'unknown'
          : 'starting'}
    label={deliveryStatusLabel(row.status)}
  />
{/snippet}
{#snippet deliveryEvent(row: Delivery)}
  <span class="capitalize">{row.eventType}</span>
{/snippet}
{#snippet deliveryChannel(row: Delivery)}
  {channelNames.get(row.channelId) ?? row.channelId}
{/snippet}
{#snippet deliveryIncident(row: Delivery)}
  {#if row.incidentId}<a class="mono" href={incidentPath(row.incidentId)}
      >{row.incidentId}</a
    >{:else}<span class="muted">test</span>{/if}
{/snippet}
{#snippet deliveryAttempts(row: Delivery)}
  <span class="num">{row.attemptCount}</span>
{/snippet}
{#snippet deliveryWhen(row: Delivery)}
  <RelativeTime value={row.updatedAt ?? row.lastAttemptAt ?? row.createdAt} />
{/snippet}
{#snippet deliveryActions(row: Delivery)}
  {#if row.status === 'permanent_failure'}
    <Button size="sm" onclick={() => retry(row)} loading={busy === row.id}>
      {#snippet icon()}<RotateCcw />{/snippet}
      Retry
    </Button>
  {:else if row.failureCode}
    <span class="muted" use:tooltip={row.failureCode}>{row.failureCode}</span>
  {/if}
{/snippet}
{#snippet deliveriesEmpty()}
  <EmptyState
    title="No deliveries yet"
    description="Every notification attempt is recorded here for 90 days."
    compact
  />
{/snippet}

<div class="stack">
  <Card
    title="Channels"
    description="Where incident notifications go. Up to 32 channels; each can filter by severity."
    padded={false}
  >
    {#snippet actions()}
      <Button
        size="sm"
        variant="primary"
        onclick={() => {
          editing = null;
          dialogOpen = true;
        }}
      >
        {#snippet icon()}<Plus />{/snippet}
        New channel
      </Button>
    {/snippet}
    {#if loading}
      <div class="loading"><Skeleton lines={3} height={48} /></div>
    {:else if error}
      <p class="error" role="alert">{error}</p>
    {:else if !channels.length}
      <EmptyState
        title="No channels yet"
        description="Add an HTTPS webhook or an SMTP mailbox to hear about incidents outside the dashboard."
      >
        {#snippet icon()}<BellRing />{/snippet}
      </EmptyState>
    {:else}
      <ul class="channels">
        {#each channels as channel (channel.id)}
          <li class:disabled={!channel.enabled}>
            <span class="kind" aria-hidden="true"
              >{#if channel.kind === 'smtp'}<Mail />{:else}<Webhook
                />{/if}</span
            >
            <div class="text">
              <div class="title-row">
                <span class="title">{channel.name}</span>
                <Badge tone="neutral"
                  >{channel.kind === 'smtp' ? 'Email' : 'Webhook'}</Badge
                >
                <Badge
                  tone={channel.minimumSeverity === 'critical'
                    ? 'critical'
                    : 'warn'}
                  >{channel.minimumSeverity === 'critical'
                    ? 'Critical only'
                    : 'Warning +'}</Badge
                >
                {#if !channel.notifyResolved}<Badge tone="neutral"
                    >no resolve notices</Badge
                  >{/if}
                {#if !channel.secretConfigured && channel.kind === 'webhook'}<Badge
                    tone="neutral">unsigned</Badge
                  >{/if}
              </div>
              <span class="target mono">{channelTarget(channel)}</span>
            </div>
            <Switch
              label={`${channel.name} enabled`}
              checked={channel.enabled}
              busy={busy === channel.id}
              onchange={(next) => toggle(channel, next)}
            />
            <Menu label={`Actions for ${channel.name}`}>
              {#snippet trigger(props)}
                <IconButton label="Channel actions" size="sm" {...props}
                  ><EllipsisVertical /></IconButton
                >
              {/snippet}
              <MenuItem onselect={() => test(channel)}
                >{#snippet icon()}<Send />{/snippet}Send test</MenuItem
              >
              <MenuItem
                onselect={() => {
                  editing = channel;
                  dialogOpen = true;
                }}>{#snippet icon()}<Pencil />{/snippet}Edit…</MenuItem
              >
              <MenuSeparator />
              <MenuItem
                danger
                onselect={() => {
                  deleting = channel;
                  confirmOpen = true;
                }}>{#snippet icon()}<Trash2 />{/snippet}Delete…</MenuItem
              >
            </Menu>
          </li>
        {/each}
      </ul>
    {/if}
  </Card>

  <Card
    title="Delivery history"
    description="Retries follow 1m, 5m, 15m, 1h, 4h, then 12h. Open and update notices coalesce for 15 seconds."
    padded={false}
  >
    <DataTable
      rows={deliveries}
      columns={deliveryColumns}
      rowKey={(row) => row.id}
      caption="Notification deliveries"
      loading={loading && !deliveries.length}
      empty={deliveriesEmpty}
      density="compact"
    />
  </Card>
</div>

<ChannelDialog
  bind:open={dialogOpen}
  channel={editing}
  onsaved={() => void load(true)}
/>
<ConfirmDialog
  bind:open={confirmOpen}
  title="Delete this channel?"
  description={deleting
    ? `${deleting.name} stops receiving notifications. Delivery history is kept.`
    : ''}
  confirmLabel="Delete channel"
  onconfirm={remove}
/>

<style>
  .stack {
    display: grid;
    gap: var(--space-5);
  }
  .loading {
    padding: var(--space-4);
  }
  .error {
    padding: var(--space-4);
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  .channels {
    margin: 0;
    padding: var(--space-2) 0;
    list-style: none;
  }
  .channels li {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
  }
  .channels li + li {
    border-top: 1px solid var(--border);
  }
  .channels li.disabled .text {
    opacity: 0.6;
  }
  .kind {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: 32px;
    height: 32px;
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    color: var(--text-2);
  }
  .kind :global(svg) {
    width: 16px;
    height: 16px;
  }
  .text {
    display: grid;
    flex: 1;
    gap: 2px;
    min-width: 0;
  }
  .title-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
  .title {
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .target {
    color: var(--text-3);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .muted {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .capitalize {
    text-transform: capitalize;
  }
</style>
