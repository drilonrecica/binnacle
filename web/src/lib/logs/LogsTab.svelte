<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import Play from '@lucide/svelte/icons/play';
  import Search from '@lucide/svelte/icons/search';
  import Square from '@lucide/svelte/icons/square';
  import {
    fetchLogs,
    followLogs,
    type LogEntry,
    type LogQuery,
    type LogRange,
  } from '../api/logs';
  import type { LiveComponent } from '../api/resources';
  import { errorMessage } from '../api/client';
  import { router } from '../router.svelte';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import Input from '../ui/Input.svelte';
  import SegmentedControl from '../ui/SegmentedControl.svelte';
  import Select from '../ui/Select.svelte';
  import Switch from '../ui/Switch.svelte';
  import { formatAbsolute } from '../ui/relative-time';
  import LogViewer from './LogViewer.svelte';
  import { LogRing, logCapacity, logPageSize } from './ring';

  /**
   * The Logs tab of a resource: the former Logs page with its source fixed
   * to this resource (DESIGN §Resources). The viewer comes first; the form
   * sits above it on one line.
   */
  let {
    resourceId,
    components = [],
  }: {
    resourceId: string;
    components?: LiveComponent[];
  } = $props();

  let container = $state(router.param('container'));
  let range = $state<LogRange>('5m');
  let from = $state<Date | null>(null);
  let to = $state<Date | null>(null);
  let search = $state('');
  let follow = $state(false);
  let colors = $state(true);
  let wrap = $state(true);
  let truncated = $state(false);
  let redaction = $state('');
  let loading = $state(false);
  let streaming = $state(false);
  let error = $state('');
  let loadedAt = $state<string | null>(null);

  // The ring is mutated in place; `version` is the only reactive signal, so a
  // streamed line costs one counter bump instead of an array copy (UI-15).
  const ring = new LogRing(logCapacity);
  let version = $state(0);
  let shown = $state(logPageSize);

  let stop: (() => void) | null = null;
  let controller: AbortController | null = null;
  let pending: LogEntry[] = [];
  let flushTimer: ReturnType<typeof setTimeout> | null = null;

  const lines = $derived.by(() => {
    void version;
    return ring.tail(shown);
  });
  const hiddenBefore = $derived.by(() => {
    void version;
    return Math.max(0, ring.size - shown);
  });
  const lineCount = $derived.by(() => {
    void version;
    return ring.size;
  });
  const componentNames = $derived(
    new Map(components.map((component) => [component.id, component.name])),
  );

  /** Applies streamed lines in batches, at most every 50 ms. */
  function flush() {
    flushTimer = null;
    if (!pending.length) return;
    ring.pushAll(
      pending.length > logCapacity ? pending.slice(-logCapacity) : pending,
    );
    pending = [];
    version = ring.version;
  }

  function enqueue(entry: LogEntry) {
    pending.push(entry);
    flushTimer ??= setTimeout(flush, 50);
  }

  function reset() {
    if (flushTimer) clearTimeout(flushTimer);
    flushTimer = null;
    pending = [];
    ring.clear();
    shown = logPageSize;
    version = ring.version;
  }

  function stopStream() {
    stop?.();
    stop = null;
    flush();
    streaming = false;
  }

  function cancel() {
    controller?.abort();
    controller = null;
    stopStream();
  }

  async function load() {
    cancel();
    error = '';
    truncated = false;
    const query: LogQuery = {
      resource: resourceId,
      container: container || undefined,
      range,
      from: from ?? undefined,
      to: to ?? undefined,
      search,
      limit: 500,
    };
    router.setQuery({ container: container || null });
    reset();
    loadedAt = new Date().toISOString();
    if (follow) {
      streaming = true;
      stop = followLogs(query, {
        onentry: enqueue,
        onend: () => {
          flush();
          streaming = false;
        },
        onerror: (message) => {
          flush();
          streaming = false;
          error = message;
        },
      });
      return;
    }
    const current = new AbortController();
    controller = current;
    loading = true;
    try {
      const result = await fetchLogs(query, current.signal);
      if (current.signal.aborted) return;
      ring.replace(result.entries);
      version = ring.version;
      truncated = result.truncated;
      redaction = result.redaction;
    } catch (reason) {
      if (!current.signal.aborted) error = errorMessage(reason);
    } finally {
      if (controller === current) {
        controller = null;
        loading = false;
      }
    }
  }

  onMount(() => {
    const at = router.param('at');
    if (at && Number.isFinite(Date.parse(at))) {
      range = 'custom';
      from = new Date(Date.parse(at) - 5 * 60_000);
      to = new Date(Date.parse(at) + 5 * 60_000);
    }
    if (container && !components.some((item) => item.id === container))
      container = '';
    void load();
  });

  onDestroy(() => {
    cancel();
    if (flushTimer) clearTimeout(flushTimer);
  });
</script>

<div class="logs">
  <div class="form" role="group" aria-label="Log source">
    {#if components.length > 1}
      <Select size="sm" bind:value={container} aria-label="Container">
        <option value="">All containers</option>
        {#each components as component (component.id)}<option
            value={component.id}>{component.name}</option
          >{/each}
      </Select>
    {/if}
    <SegmentedControl
      label="Log range"
      size="sm"
      bind:value={range}
      options={[
        { value: '5m', label: '5m' },
        { value: '30m', label: '30m' },
        { value: '1h', label: '1h' },
        ...(range === 'custom'
          ? [{ value: 'custom' as LogRange, label: 'Around event' }]
          : []),
      ]}
    />
    <div class="search">
      <Input
        type="search"
        bind:value={search}
        maxlength={256}
        placeholder="Search: error, request id, path…"
        aria-label="Search log lines (literal text, case-insensitive)"
        onkeydown={(event) => event.key === 'Enter' && void load()}
      >
        {#snippet leading()}<Search />{/snippet}
      </Input>
    </div>
    <label class="toggle"
      ><Switch label="Follow new lines" bind:checked={follow} />Follow</label
    >
    <label class="toggle"
      ><Switch label="Severity colours" bind:checked={colors} />Colours</label
    >
    <label class="toggle"
      ><Switch label="Wrap long lines" bind:checked={wrap} />Wrap</label
    >
    {#if streaming}
      <Button size="sm" variant="danger" onclick={stopStream}>
        {#snippet icon()}<Square />{/snippet}
        Stop
      </Button>
    {:else}
      <Button size="sm" variant="primary" onclick={load} {loading}>
        {#snippet icon()}<Play />{/snippet}
        {follow ? 'Follow' : 'Load'}
      </Button>
    {/if}
  </div>

  <Card padded={false}>
    {#if error}
      <p class="banner error" role="alert">{error}</p>
    {/if}
    {#if truncated}
      <p class="banner warn" role="status">
        Output reached the configured line or byte limit. Narrow the range or
        search to see more.
      </p>
    {/if}
    {#if streaming}
      <p class="banner info" role="status">
        Following live output. The stream ends automatically after 30 minutes.
      </p>
    {/if}
    <LogViewer
      {lines}
      {hiddenBefore}
      onshowearlier={() => (shown += logPageSize)}
      following={streaming}
      {wrap}
      {colors}
      {search}
      {loading}
      {componentNames}
    />
    {#snippet footer()}
      <p class="note">
        {#if loadedAt}
          {streaming ? 'Streaming since' : 'Loaded'}
          {formatAbsolute(loadedAt)} · {lineCount}
          {lineCount === 1 ? 'line' : 'lines'}{lineCount >= logCapacity
            ? ` (newest ${logCapacity} kept)`
            : ''}.
        {/if}
        Logs are not stored. Redaction is best-effort{redaction &&
        redaction !== 'best-effort'
          ? ` (${redaction})`
          : ''}; avoid logging secrets.
      </p>
      {#if range === 'custom' && from && to}
        <Badge tone="info">{formatAbsolute(from)} – {formatAbsolute(to)}</Badge>
      {/if}
    {/snippet}
  </Card>
</div>

<style>
  .logs {
    display: grid;
    gap: var(--space-3);
  }
  .form {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
  }
  .form :global(.select-wrap) {
    width: auto;
  }
  .search {
    flex: 1 1 220px;
    min-width: 0;
    max-width: 360px;
  }
  .toggle {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .banner {
    padding: var(--space-2) var(--space-4);
    border-bottom: 1px solid var(--line);
    font-size: var(--text-meta);
  }
  .banner.error {
    color: var(--critical-fg);
    background: var(--critical-bg);
  }
  .banner.warn {
    color: var(--warn-fg);
    background: var(--warn-bg);
  }
  .banner.info {
    color: var(--info-fg);
    background: var(--info-bg);
  }
  .note {
    margin-right: auto;
    color: var(--muted);
    font-size: var(--text-mono);
  }
</style>
