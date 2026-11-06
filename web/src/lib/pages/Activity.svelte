<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import type { LiveStore } from '../live.svelte';
  import {
    eventFamilies,
    eventFamily,
    isEventFamily,
    listEvents,
    type EventFamily,
    type EventQuery,
    type HistoricalEvent,
  } from '../api/events';
  import { errorMessage } from '../api/client';
  import { eventRangeFor, type EventRangeKey } from '../events';
  import { router } from '../router.svelte';
  import EventList from '../activity/EventList.svelte';
  import { liveRow, mergeTimeline, timelineCap } from '../activity/timeline';
  import ResourcePicker from '../resources/ResourcePicker.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import Field from '../ui/Field.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import SegmentedControl from '../ui/SegmentedControl.svelte';
  import Select from '../ui/Select.svelte';
  import Switch from '../ui/Switch.svelte';

  let { live }: { live: LiveStore } = $props();

  const ranges: EventRangeKey[] = ['1h', '6h', '24h', '7d'];
  const severities = ['info', 'warning', 'critical'] as const;
  type Severity = 'all' | (typeof severities)[number];

  function initialRange(): EventRangeKey {
    const value = router.param('range');
    return ranges.find((key) => key === value) ?? '24h';
  }
  function initialSeverity(): Severity {
    const value = router.param('severity');
    return severities.find((key) => key === value) ?? 'all';
  }
  function initialFamily(): EventFamily | '' {
    const value = router.param('family');
    return isEventFamily(value) ? value : '';
  }

  let range = $state<EventRangeKey>(initialRange());
  let severity = $state<Severity>(initialSeverity());
  let family = $state<EventFamily | ''>(initialFamily());
  let resourceId = $state(router.param('resource'));
  let liveOn = $state(true);

  /** Live and history together, newest first, each server id once. */
  let rows = $state.raw<HistoricalEvent[]>([]);
  /** Live rows held back while the reader is scrolled down. */
  let pending = $state.raw<HistoricalEvent[]>([]);
  let capped = $state(false);
  let loading = $state(true);
  let loadingMore = $state(false);
  let exhausted = $state(false);
  let error = $state('');
  let seenLiveSeq = 0;
  let highlight = $state.raw(new Set<string>());
  /** One controller per filter set: a new load or leaving the page aborts it. */
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

  function query(before?: string): EventQuery {
    const { from, to } = eventRangeFor(range);
    return {
      from,
      to,
      severity: severity === 'all' ? undefined : severity,
      family: family || undefined,
      resourceId: resourceId || undefined,
      limit: pageSize,
      before,
    };
  }

  function apply(incoming: HistoricalEvent[]) {
    const merged = mergeTimeline(rows, incoming, timelineCap);
    rows = merged.rows;
    capped ||= merged.capped;
  }

  async function load() {
    controller?.abort();
    const current = new AbortController();
    controller = current;
    loading = true;
    loadingMore = false;
    error = '';
    exhausted = false;
    capped = false;
    pending = [];
    rows = [];
    seenLiveSeq = live.events.at(-1)?.seq ?? 0;
    router.setQuery({
      range: range === '24h' ? null : range,
      severity: severity === 'all' ? null : severity,
      family: family || null,
      resource: resourceId || null,
    });
    try {
      const page = await listEvents(query(), current.signal);
      if (controller !== current) return;
      apply(page);
      exhausted = page.length < pageSize;
    } catch (reason) {
      if (controller === current && !current.signal.aborted)
        error = errorMessage(reason);
    } finally {
      if (controller === current) loading = false;
    }
  }

  async function loadMore() {
    const current = controller;
    const last = rows.at(-1);
    if (!current || !last || loadingMore || exhausted || capped) return;
    loadingMore = true;
    try {
      const page = await listEvents(query(last.id), current.signal);
      if (controller !== current) return;
      apply(page);
      exhausted = page.length < pageSize;
    } catch (reason) {
      if (controller === current && !current.signal.aborted)
        error = errorMessage(reason);
    } finally {
      if (controller === current) loadingMore = false;
    }
  }

  // Live events arrive through the SSE store. They carry the stored id, so a
  // live event that the history page also returned is shown once. They join
  // at the top without moving the list: a reader scrolled down gets the
  // "N new" button instead.
  $effect(() => {
    const events = live.events;
    untrack(() => {
      const fresh = events.filter((event) => event.seq > seenLiveSeq);
      if (!fresh.length) return;
      seenLiveSeq = fresh[fresh.length - 1].seq;
      if (!liveOn) return;
      const held = new Set(rows.map((row) => row.id));
      const incoming = fresh
        .map((event) => liveRow(event))
        .filter((row) => matches(row) && !held.has(row.id));
      if (!incoming.length) return;
      if (window.scrollY > 240) {
        pending = mergeTimeline(pending, incoming, timelineCap).rows;
      } else {
        reveal(incoming);
      }
    });
  });

  function reveal(incoming: HistoricalEvent[]) {
    apply(incoming);
    const ids = new Set(incoming.map((row) => row.id));
    highlight = new Set([...highlight, ...ids]);
    window.setTimeout(() => {
      highlight = new Set([...highlight].filter((id) => !ids.has(id)));
    }, 1600);
  }

  function flushPending() {
    const incoming = pending;
    pending = [];
    reveal(incoming);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  onMount(() => {
    void load();
    return () => controller?.abort();
  });
</script>

<PageHeader
  title="Changes"
  description="Everything that changed on this server: deploys, container lifecycle and health, collectors, alerts, and system events."
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
    <option value="">All families</option>
    {#each eventFamilies as item (item.value)}<option value={item.value}
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
        variant="quiet"
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
    events={rows}
    grouped
    {loading}
    {error}
    resourceNames={names}
    highlightIds={highlight}
    emptyTitle="No events match"
    emptyDescription="Widen the time range or clear a filter."
  />
  {#snippet footer()}
    {#if !loading && rows.length && capped}
      <span class="end" role="status"
        >Showing the newest {timelineCap.toLocaleString()} events. Narrow the range
        or add a filter to see older ones.</span
      >
    {:else if !loading && rows.length && !exhausted}
      <Button onclick={loadMore} loading={loadingMore}>Load older events</Button
      >
    {:else if !loading && rows.length}
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
    color: var(--muted);
    font-size: var(--text-mono);
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
