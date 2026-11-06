<script lang="ts">
  import { onMount } from 'svelte';
  import BellOff from '@lucide/svelte/icons/bell-off';
  import Eye from '@lucide/svelte/icons/eye';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import type { LiveStore } from '../live.svelte';
  import {
    getIncident,
    incidentTitle,
    targetLabel,
    type Alert,
    type Delivery,
  } from '../api/incidents';
  import { listChannels, retryDelivery, type Channel } from '../api/channels';
  import { deliveryStatusLabel } from '../api/channels';
  import type { SilenceScope } from '../api/silences';
  import { errorMessage, isApiError } from '../api/client';
  import { resourcePath } from '../router';
  import { ruleInfo } from '../alerts/rule-catalog';
  import { seen } from '../alerts/seen.svelte';
  import SilenceDialog from '../alerts/SilenceDialog.svelte';
  import { TabSource } from '../alerts/tab-source.svelte';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import DataTable, { type Column } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import ErrorState from '../ui/ErrorState.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import StaleBanner from '../ui/StaleBanner.svelte';
  import Status from '../ui/Status.svelte';
  import StatusPill from '../ui/StatusPill.svelte';
  import { toasts } from '../ui/toast.svelte';
  import { formatAbsolute, formatClock, formatSpan } from '../ui/relative-time';
  import { severityLabel, severityTone } from '../ui/status';

  let {
    live,
    id,
    viewer = '',
  }: {
    live: LiveStore;
    id: string;
    /** The signed-in username, recorded by the local "seen by" marker. */
    viewer?: string;
  } = $props();

  const source = new TabSource((signal) => getIncident(id, signal));
  let channels = $state<Channel[]>([]);
  let notFound = $state(false);
  let retrying = $state<string | null>(null);
  let silenceOpen = $state(false);
  let controller: AbortController | undefined;

  const incident = $derived(source.data);
  const mark = $derived(incident ? seen.get(incident.id) : undefined);

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
  // Stored titles embed the raw target id; show the resource name (UI-18).
  const title = $derived(
    incident ? incidentTitle(incident, targetName) : 'Incident',
  );
  const silenceTarget = $derived<{ scope: SilenceScope; id: string }>(
    incident?.targetType === 'resource'
      ? { scope: 'resource', id: incident.targetId }
      : { scope: 'server', id: '' },
  );

  async function load() {
    const signal = controller?.signal;
    try {
      const value = await getIncident(id, signal);
      if (signal?.aborted) return;
      source.settle({ status: 'fulfilled', value });
    } catch (reason) {
      if (signal?.aborted) return;
      if (isApiError(reason) && reason.status === 404 && !source.data)
        notFound = true;
      else source.settle({ status: 'rejected', reason });
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
    controller = new AbortController();
    void load();
    void listChannels()
      .then((value) => (channels = value))
      .catch(() => {
        /* Channel names are decoration; ids are shown instead. */
      });
    // A resolved incident no longer changes, so only open ones are polled.
    const timer = window.setInterval(() => {
      if (document.visibilityState !== 'visible') return;
      if (source.data?.status === 'resolved') return;
      void load();
    }, 15_000);
    return () => {
      window.clearInterval(timer);
      controller?.abort();
    };
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

{#snippet alertCard(row: Alert)}
  <div class="card">
    <div class="card-top">
      <span>{row.message}</span>
      {@render alertStatus(row)}
    </div>
    <div class="card-meta">
      <Badge tone={severityTone(row.severity)}
        >{severityLabel(row.severity)}</Badge
      >
      <span>{ruleInfo(row.family, row.family).title}</span>
      <span>started <RelativeTime value={row.startedAt} /></span>
      {#if row.resolvedAt}<span
          >resolved <RelativeTime value={row.resolvedAt} /></span
        >{/if}
    </div>
  </div>
{/snippet}
{#snippet deliveryCard(row: Delivery)}
  <div class="card">
    <div class="card-top">
      {@render deliveryChannel(row)}
      {@render deliveryStatus(row)}
    </div>
    <div class="card-meta">
      <span class="capitalize">{row.eventType}</span>
      <span
        >{row.attemptCount}
        {row.attemptCount === 1 ? 'attempt' : 'attempts'}</span
      >
      {@render deliveryWhen(row)}
    </div>
    {@render deliveryActions(row)}
  </div>
{/snippet}

{#if incident}
  {@const current = incident}
  <PageHeader
    {title}
    crumbs={[{ label: 'Incidents', href: '/incidents' }]}
    headingId="incident-title"
  >
    {#snippet meta()}
      {#if current.status === 'open'}
        <Status
          tone={severityTone(current.severity)}
          label={`Open · ${severityLabel(current.severity)}`}
        />
      {:else}
        <Status tone="ok" label="Resolved" />
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
    {#snippet actions()}
      {#if current.status === 'open'}
        <Button onclick={() => (silenceOpen = true)}>
          {#snippet icon()}<BellOff />{/snippet}
          Silence…
        </Button>
      {/if}
    {/snippet}
  </PageHeader>

  <div class="stack">
    {#if source.stale}
      <StaleBanner
        message={`This incident could not be refreshed: ${source.error}`}
        since={source.loadedAt}
      >
        <Button size="sm" variant="quiet" onclick={() => void load()}
          >Try again</Button
        >
      </StaleBanner>
    {/if}

    <section class="seen" aria-labelledby="incident-seen-title">
      <h2 id="incident-seen-title" class="sr-only">Seen by</h2>
      <Eye aria-hidden="true" />
      {#if mark}
        <p>
          Seen by <strong>{mark.by}</strong> at
          <time datetime={mark.at}
            >{formatClock(mark.at, undefined, false)}</time
          >
          <span class="muted"
            >· {formatAbsolute(mark.at)} · this browser only</span
          >
        </p>
        <Button size="sm" variant="quiet" onclick={() => seen.clear(current.id)}
          >Clear</Button
        >
      {:else}
        <p class="muted">
          Not marked as seen in this browser. Marking it tells anyone using this
          browser that someone is on it; it is not sent to the server.
        </p>
        <Button size="sm" onclick={() => seen.mark(current.id, viewer)}
          >Mark as seen</Button
        >
      {/if}
    </section>

    <Card
      title="Alerts in this incident"
      description="The incident stays open while any member alert is firing and resolves itself when the last one clears."
      padded={false}
    >
      <DataTable
        rows={current.alerts ?? []}
        columns={alertColumns}
        rowKey={(row) => row.id}
        caption="Member alerts"
        mobileCard={alertCard}
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
        rows={current.deliveries ?? []}
        columns={deliveryColumns}
        rowKey={(row) => row.id}
        caption="Notification deliveries"
        mobileCard={deliveryCard}
      >
        {#snippet empty()}<EmptyState
            title="No notifications sent"
            description="Add a channel under Settings › Notifications to be told about incidents."
            compact
            ><Button href="/settings/notifications"
              >Notification settings</Button
            ></EmptyState
          >{/snippet}
      </DataTable>
    </Card>
  </div>

  <SilenceDialog
    bind:open={silenceOpen}
    snapshot={live.snapshot}
    scope={silenceTarget.scope}
    scopeId={silenceTarget.id}
  />
{:else if notFound}
  <PageHeader
    title="Incident not found"
    crumbs={[{ label: 'Incidents', href: '/incidents' }]}
  />
  <EmptyState
    title="This incident does not exist"
    description="It may have been cleaned up after its retention period."
  >
    <Button href="/incidents" variant="primary">Back to incidents</Button>
  </EmptyState>
{:else if source.failed}
  <PageHeader
    title="Incident"
    crumbs={[{ label: 'Incidents', href: '/incidents' }]}
  />
  <ErrorState
    title="This incident could not be loaded"
    description={source.error}
    onretry={() => void load()}
  />
{:else}
  <div class="loading" role="status" aria-label="Loading incident">
    <Skeleton height={32} width={320} />
    <Skeleton height={200} />
  </div>
{/if}

<style>
  .stack {
    display: grid;
    gap: var(--space-4);
  }
  .target {
    font-family: var(--font-mono);
    font-weight: 500;
  }
  .muted {
    color: var(--muted);
    font-size: var(--text-mono);
  }
  .alert-message {
    display: grid;
    gap: 2px;
    white-space: normal;
  }
  .capitalize {
    text-transform: capitalize;
  }
  .seen {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--line);
    border-radius: var(--radius-card);
    background: var(--panel);
    font-size: var(--text-meta);
  }
  .seen > :global(svg) {
    width: 16px;
    height: 16px;
    color: var(--muted);
  }
  .seen p {
    flex: 1 1 auto;
    min-width: 0;
  }
  .card {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    justify-items: start;
  }
  .card-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    width: 100%;
  }
  .card-meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-3);
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .loading {
    display: grid;
    gap: var(--space-4);
  }
</style>
