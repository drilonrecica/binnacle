<script lang="ts">
  import { onMount } from 'svelte';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import type { LiveStore } from '../live.svelte';
  import {
    getIncident,
    targetLabel,
    type Alert,
    type Delivery,
    type Incident,
  } from '../api/incidents';
  import { listChannels, retryDelivery, type Channel } from '../api/channels';
  import { deliveryStatusLabel } from '../api/channels';
  import { errorMessage, isApiError } from '../api/client';
  import { resourcePath } from '../router';
  import { ruleInfo } from '../alerts/rule-catalog';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import DataTable, { type Column } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import StatusPill from '../ui/StatusPill.svelte';
  import { toasts } from '../ui/toast.svelte';
  import { formatAbsolute, formatSpan } from '../ui/relative-time';
  import { severityLabel, severityTone } from '../ui/status';

  let { live, id }: { live: LiveStore; id: string } = $props();

  let incident = $state<Incident | null>(null);
  let channels = $state<Channel[]>([]);
  let notFound = $state(false);
  let error = $state('');
  let retrying = $state<string | null>(null);

  const names = $derived(
    new Map(
      (live.snapshot?.resources ?? []).map((resource) => [
        resource.id,
        resource.name,
      ]),
    ),
  );
  const channelNames = $derived(
    new Map(channels.map((channel) => [channel.id, channel.name])),
  );
  const targetName = $derived(
    incident
      ? incident.targetType === 'resource'
        ? (names.get(incident.targetId) ?? incident.targetId)
        : targetLabel(incident.targetType, incident.targetId)
      : '',
  );
  const title = $derived(
    incident
      ? incident.targetType === 'resource' &&
        incident.title.includes(incident.targetId)
        ? `Incident on ${targetName}`
        : incident.title
      : 'Incident',
  );

  async function load() {
    try {
      incident = await getIncident(id);
    } catch (reason) {
      if (isApiError(reason) && reason.status === 404) notFound = true;
      else error = errorMessage(reason);
    }
  }

  async function retry(delivery: Delivery) {
    retrying = delivery.id;
    try {
      await retryDelivery(delivery.id);
      toasts.success('Retry queued');
      await load();
    } catch (reason) {
      toasts.error('Retry failed', { description: errorMessage(reason) });
    } finally {
      retrying = null;
    }
  }

  onMount(() => {
    void load();
    void listChannels()
      .then((value) => (channels = value))
      .catch(() => {
        /* Channel names are decoration. */
      });
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') void load();
    }, 15_000);
    return () => window.clearInterval(timer);
  });

  const alertColumns: Column<Alert>[] = [
    { key: 'status', label: 'Status', width: '110px', cell: alertStatus },
    { key: 'message', label: 'Alert', cell: alertMessage },
    {
      key: 'started',
      label: 'Started',
      align: 'right',
      width: '140px',
      cell: alertStarted,
    },
    {
      key: 'resolved',
      label: 'Resolved',
      align: 'right',
      width: '140px',
      hideBelow: 900,
      cell: alertResolved,
    },
  ];
  const deliveryColumns: Column<Delivery>[] = [
    { key: 'status', label: 'Status', width: '120px', cell: deliveryStatus },
    { key: 'event', label: 'Event', cell: deliveryEvent },
    { key: 'channel', label: 'Channel', cell: deliveryChannel },
    {
      key: 'attempts',
      label: 'Attempts',
      align: 'right',
      width: '90px',
      hideBelow: 900,
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

{#snippet alertStatus(row: Alert)}
  <StatusPill
    status={row.status === 'firing'
      ? row.severity === 'critical'
        ? 'down'
        : 'degraded'
      : 'healthy'}
    label={row.status === 'firing' ? 'Firing' : 'Resolved'}
  />
{/snippet}
{#snippet alertMessage(row: Alert)}
  <div class="alert-message">
    <span>{row.message}</span>
    <span class="muted"
      ><Badge tone={severityTone(row.severity)}
        >{severityLabel(row.severity)}</Badge
      >
      {ruleInfo(row.family, row.family).title}</span
    >
  </div>
{/snippet}
{#snippet alertStarted(row: Alert)}
  <RelativeTime value={row.startedAt} />
{/snippet}
{#snippet alertResolved(row: Alert)}
  <RelativeTime value={row.resolvedAt} />
{/snippet}
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
  <a href="/settings/notifications"
    >{channelNames.get(row.channelId) ?? row.channelId}</a
  >
{/snippet}
{#snippet deliveryAttempts(row: Delivery)}
  <span class="num">{row.attemptCount}</span>
{/snippet}
{#snippet deliveryWhen(row: Delivery)}
  <RelativeTime value={row.updatedAt ?? row.lastAttemptAt ?? row.createdAt} />
{/snippet}
{#snippet deliveryActions(row: Delivery)}
  {#if row.status === 'permanent_failure'}
    <Button size="sm" onclick={() => retry(row)} loading={retrying === row.id}>
      {#snippet icon()}<RotateCcw />{/snippet}
      Retry
    </Button>
  {:else if row.failureCode}
    <span class="muted">{row.failureCode}</span>
  {/if}
{/snippet}

{#if incident}
  {@const current = incident}
  <PageHeader
    {title}
    crumbs={[{ label: 'Alerts', href: '/alerts' }]}
    headingId="incident-title"
  >
    {#snippet meta()}
      {#if current.status === 'open'}
        <StatusPill
          status={current.severity === 'critical' ? 'down' : 'degraded'}
          label={`Open · ${severityLabel(current.severity)}`}
        />
      {:else}
        <StatusPill status="healthy" label="Resolved" />
      {/if}
      {#if current.targetType === 'resource'}
        <a class="target" href={resourcePath(current.targetId)}>{targetName}</a>
      {:else}
        <span class="target">{targetName}</span>
      {/if}
      <span class="muted">opened {formatAbsolute(current.openedAt)}</span>
      {#if current.resolvedAt}
        <span class="muted"
          >resolved after {formatSpan(
            current.openedAt,
            current.resolvedAt,
          )}</span
        >
      {:else}
        <span class="muted">open for {formatSpan(current.openedAt)}</span>
      {/if}
    {/snippet}
  </PageHeader>

  <div class="stack">
    <Card
      title="Alerts in this incident"
      description="The incident stays open while any member alert is firing and resolves itself when the last one clears."
      padded={false}
    >
      <DataTable
        rows={incident.alerts ?? []}
        columns={alertColumns}
        rowKey={(row) => row.id}
        caption="Member alerts"
      >
        {#snippet empty()}<EmptyState
            title="No alerts recorded"
            compact
          />{/snippet}
      </DataTable>
    </Card>
    <Card
      title="Notifications"
      description="Deliveries for this incident. Failed deliveries retry automatically; permanent failures can be retried by hand."
      padded={false}
    >
      <DataTable
        rows={incident.deliveries ?? []}
        columns={deliveryColumns}
        rowKey={(row) => row.id}
        caption="Notification deliveries"
      >
        {#snippet empty()}<EmptyState
            title="No notifications sent"
            description="Add a channel under Settings → Notifications to be told about incidents."
            compact
            ><Button href="/settings/notifications"
              >Notification settings</Button
            ></EmptyState
          >{/snippet}
      </DataTable>
    </Card>
  </div>
{:else if notFound}
  <PageHeader
    title="Incident not found"
    crumbs={[{ label: 'Alerts', href: '/alerts' }]}
  />
  <EmptyState
    title="This incident does not exist"
    description="It may have been cleaned up after its retention period."
  >
    <Button href="/alerts" variant="primary">Back to alerts</Button>
  </EmptyState>
{:else if error}
  <PageHeader
    title="Incident"
    crumbs={[{ label: 'Alerts', href: '/alerts' }]}
  />
  <p class="error" role="alert">{error}</p>
{:else}
  <div class="loading" role="status" aria-label="Loading incident">
    <Skeleton height={32} width={320} />
    <Skeleton height={200} />
  </div>
{/if}

<style>
  .stack {
    display: grid;
    gap: var(--space-5);
  }
  .target {
    font-family: var(--font-mono);
    font-weight: 500;
  }
  .muted {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .alert-message {
    display: grid;
    gap: 2px;
    white-space: normal;
  }
  .capitalize {
    text-transform: capitalize;
  }
  .error {
    color: var(--critical-fg);
  }
  .loading {
    display: grid;
    gap: var(--space-4);
  }
</style>
