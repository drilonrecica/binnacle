<script lang="ts">
  import { onMount } from 'svelte';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import type { LiveStore } from '../live.svelte';
  import {
    eventFamily,
    listEvents,
    type EventFamily,
    type HistoricalEvent,
  } from '../api/events';
  import { errorMessage } from '../api/client';
  import { eventRangeFor, type EventRangeKey } from '../events';
  import { router } from '../router.svelte';
  import EventList from '../activity/EventList.svelte';
  import ResourcePicker from '../resources/ResourcePicker.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import Field from '../ui/Field.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import SegmentedControl from '../ui/SegmentedControl.svelte';
  import Select from '../ui/Select.svelte';
  import Switch from '../ui/Switch.svelte';

  let { live }: { live: LiveStore } = $props();

  const families: Array<{
    value: EventFamily | '';
    label: string;
    types: string[];
  }> = [
    { value: '', label: 'All events', types: [] },
    {
      value: 'containers',
      label: 'Containers',
      types: [
        'container_start',
        'container_stop',
        'container_die',
        'container_destroy',
        'container_create',
        'container_rename',
        'container_oom',
        'container_restart',
        'container_health_status_change',
        'resource_archived',
      ],
    },
    {
      value: 'deployments',
      label: 'Deployments',
      types: ['deployment', 'deployment_likely', 'container_replacement'],
    },
    {
      value: 'collectors',
      label: 'Collectors',
      types: ['collector_degraded', 'collector_down'],
    },
    {
      value: 'storage',
      label: 'Storage',
      types: [
        'persistence_degraded',
        'persistence_gap',
        'persistence_emergency',
        'persistence_resumed',
      ],
    },
    {
      value: 'alerts',
      label: 'Alerts',
      types: ['alert_triggered', 'alert_repeated', 'alert_resolved'],
    },
  ];

  let range = $state<EventRangeKey>(
    (router.param('range') as EventRangeKey) || '24h',
  );
  let severity = $state<'all' | 'info' | 'warning' | 'critical'>(
    (router.param('severity') as 'info' | 'warning' | 'critical') || 'all',
  );
  let family = $state<EventFamily | ''>(
    (router.param('family') as EventFamily) || '',
  );
  let resourceId = $state(router.param('resource'));
  let liveOn = $state(true);

  let history = $state<HistoricalEvent[]>([]);
  let loading = $state(true);
  let loadingMore = $state(false);
  let exhausted = $state(false);
  let error = $state('');
  let seenLiveId = $state(0);
  let pending = $state<HistoricalEvent[]>([]);
  let shown = $state<HistoricalEvent[]>([]);
  let highlight = $state(new Set<string>());
  let controller: AbortController | undefined;

  const names = $derived(
    new Map(
      (live.snapshot?.resources ?? []).map((resource) => [
        resource.id,
        resource.name,
      ]),
    ),
  );
  const pageSize = 100;

  function matches(event: HistoricalEvent) {
    if (severity !== 'all' && event.severity !== severity) return false;
    if (family && eventFamily(event.type) !== family) return false;
    if (resourceId && event.resourceId !== resourceId) return false;
    return true;
  }

  function fromLive(event: LiveStore['events'][number]): HistoricalEvent {
    return {
      id: `live-${event.id}`,
      ts: event.at ?? new Date().toISOString(),
      type: event.type,
      severity: (event.severity as HistoricalEvent['severity']) || 'info',
      summary: event.message,
      resourceId: event.resourceId,
      containerInstanceId: event.containerInstanceId,
      source: 'live',
    };
  }

  async function load() {
    controller?.abort();
    controller = new AbortController();
    loading = true;
    error = '';
    exhausted = false;
    pending = [];
    shown = [];
    seenLiveId = live.events.at(-1)?.id ?? 0;
    router.setQuery({
      range: range === '24h' ? null : range,
      severity: severity === 'all' ? null : severity,
      family: family || null,
      resource: resourceId || null,
    });
    try {
      const { from, to } = eventRangeFor(range);
      const types = families.find((item) => item.value === family)?.types ?? [];
      history = await listEvents(
        {
          from,
          to,
          severity: severity === 'all' ? undefined : severity,
          type: types.join(',') || undefined,
          resourceId: resourceId || undefined,
          limit: pageSize,
        },
        controller.signal,
      );
      exhausted = history.length < pageSize;
    } catch (reason) {
      if (!controller.signal.aborted) error = errorMessage(reason);
    } finally {
      loading = false;
    }
  }

  async function loadMore() {
    const last = history.at(-1);
    if (!last || loadingMore || exhausted) return;
    loadingMore = true;
    try {
      const { from, to } = eventRangeFor(range);
      const types = families.find((item) => item.value === family)?.types ?? [];
      const more = await listEvents({
        from,
        to,
        severity: severity === 'all' ? undefined : severity,
        type: types.join(',') || undefined,
        resourceId: resourceId || undefined,
        limit: pageSize,
        before: last.id,
      });
      history = [...history, ...more];
      exhausted = more.length < pageSize;
    } catch (reason) {
      error = errorMessage(reason);
    } finally {
      loadingMore = false;
    }
  }

  // Live events arrive through the SSE store; queue them when the reader has
  // scrolled away so the list never jumps under their pointer.
  $effect(() => {
    const fresh = live.events.filter((event) => event.id > seenLiveId);
    if (!fresh.length) return;
    seenLiveId = fresh[fresh.length - 1].id;
    if (!liveOn) return;
    const rows = fresh.map(fromLive).filter(matches).reverse();
    if (!rows.length) return;
    if (window.scrollY > 240) {
      pending = [...rows, ...pending];
    } else {
      reveal(rows);
    }
  });

  function reveal(rows: HistoricalEvent[]) {
    shown = [...rows, ...shown];
    highlight = new Set([...highlight, ...rows.map((row) => row.id)]);
    window.setTimeout(() => {
      highlight = new Set(
        [...highlight].filter((id) => !rows.some((row) => row.id === id)),
      );
    }, 1600);
  }

  function flushPending() {
    const rows = pending;
    pending = [];
    reveal(rows);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  const events = $derived([...shown, ...history]);

  onMount(() => {
    void load();
    return () => controller?.abort();
  });
</script>

<PageHeader
  title="Activity"
  description="Everything that changed on this server: container lifecycle, deployments, collector state, storage, and alerts."
>
  {#snippet actions()}
    <Field label="Live" inline id="activity-live">
      {#snippet children({ id })}
        <Switch {id} bind:checked={liveOn} />
      {/snippet}
    </Field>
  {/snippet}
</PageHeader>

<div class="toolbar">
  <SegmentedControl
    label="Time range"
    size="sm"
    bind:value={range}
    options={[
      { value: '1h', label: '1h' },
      { value: '6h', label: '6h' },
      { value: '24h', label: '24h' },
      { value: '7d', label: '7d' },
    ]}
    onchange={() => void load()}
  />
  <SegmentedControl
    label="Severity"
    size="sm"
    bind:value={severity}
    options={[
      { value: 'all', label: 'All' },
      { value: 'info', label: 'Info' },
      { value: 'warning', label: 'Warning' },
      { value: 'critical', label: 'Critical' },
    ]}
    onchange={() => void load()}
  />
  <Select
    size="sm"
    bind:value={family}
    aria-label="Event family"
    onchange={() => void load()}
  >
    {#each families as item (item.value)}<option value={item.value}
        >{item.label}</option
      >{/each}
  </Select>
  <div class="picker">
    <ResourcePicker
      snapshot={live.snapshot}
      bind:value={resourceId}
      onchange={() => void load()}
    />
    {#if resourceId}
      <Button
        size="sm"
        variant="ghost"
        onclick={() => {
          resourceId = '';
          void load();
        }}>Clear</Button
      >
    {/if}
  </div>
</div>

{#if pending.length}
  <div class="pending">
    <Button size="sm" variant="primary" onclick={flushPending}>
      {#snippet icon()}<ArrowUp />{/snippet}
      {pending.length} new {pending.length === 1 ? 'event' : 'events'}
    </Button>
  </div>
{/if}

<Card padded={false}>
  <EventList
    {events}
    {loading}
    {error}
    resourceNames={names}
    highlightIds={highlight}
    emptyTitle="No events match"
    emptyDescription="Widen the time range or clear a filter."
  />
  {#snippet footer()}
    {#if !loading && history.length && !exhausted}
      <Button onclick={loadMore} loading={loadingMore}>Load older events</Button
      >
    {:else if !loading && history.length}
      <span class="end">No older events in this range</span>
    {/if}
  {/snippet}
</Card>

<style>
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    margin-bottom: var(--space-4);
  }
  .toolbar :global(.select-wrap) {
    width: auto;
  }
  .picker {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: min(100%, 320px);
  }
  .end {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .pending {
    position: sticky;
    top: calc(var(--topbar-h) + var(--space-3));
    z-index: 20;
    display: flex;
    justify-content: center;
    margin-bottom: var(--space-3);
  }
</style>
