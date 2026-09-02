<script lang="ts">
  import { onMount } from 'svelte';
  import Play from '@lucide/svelte/icons/play';
  import Square from '@lucide/svelte/icons/square';
  import Search from '@lucide/svelte/icons/search';
  import type { LiveStore } from '../live.svelte';
  import {
    fetchLogs,
    followLogs,
    type LogEntry,
    type LogRange,
  } from '../api/logs';
  import { errorMessage } from '../api/client';
  import { router } from '../router.svelte';
  import { resourcePath } from '../router';
  import LogViewer from '../logs/LogViewer.svelte';
  import ResourcePicker from '../resources/ResourcePicker.svelte';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import SegmentedControl from '../ui/SegmentedControl.svelte';
  import Select from '../ui/Select.svelte';
  import Switch from '../ui/Switch.svelte';
  import { formatAbsolute } from '../ui/relative-time';

  let { live }: { live: LiveStore } = $props();

  let resourceId = $state(router.param('resource'));
  let container = $state(router.param('container'));
  let range = $state<LogRange>('5m');
  let from = $state<Date | null>(null);
  let to = $state<Date | null>(null);
  let search = $state('');
  let follow = $state(false);
  let colors = $state(true);
  let wrap = $state(true);
  let entries = $state<LogEntry[]>([]);
  let truncated = $state(false);
  let redaction = $state('');
  let loading = $state(false);
  let streaming = $state(false);
  let error = $state('');
  let loadedAt = $state<string | null>(null);
  let stop: (() => void) | null = null;

  const selected = $derived(
    live.snapshot?.resources.find((resource) => resource.id === resourceId) ??
      null,
  );
  const components = $derived(selected?.components ?? []);
  const componentNames = $derived(
    new Map(components.map((component) => [component.id, component.name])),
  );

  function stopStream() {
    stop?.();
    stop = null;
    streaming = false;
  }

  async function load() {
    stopStream();
    error = '';
    truncated = false;
    if (!resourceId && !container) {
      error = 'Choose a resource first.';
      return;
    }
    const query = {
      resource: resourceId,
      container: container || undefined,
      range,
      from: from ?? undefined,
      to: to ?? undefined,
      search,
      limit: 500,
    };
    router.setQuery({
      resource: resourceId || null,
      container: container || null,
    });
    if (follow) {
      entries = [];
      streaming = true;
      loadedAt = new Date().toISOString();
      stop = followLogs(query, {
        onentry: (entry) => {
          entries = [...entries.slice(-4999), entry];
        },
        onend: () => {
          streaming = false;
        },
        onerror: (message) => {
          streaming = false;
          error = message;
        },
      });
      return;
    }
    loading = true;
    try {
      const result = await fetchLogs(query);
      entries = result.entries;
      truncated = result.truncated;
      redaction = result.redaction;
      loadedAt = new Date().toISOString();
    } catch (reason) {
      error = errorMessage(reason);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    const at = router.param('at');
    if (at && Number.isFinite(Date.parse(at))) {
      range = 'custom';
      from = new Date(Date.parse(at) - 5 * 60_000);
      to = new Date(Date.parse(at) + 5 * 60_000);
    }
    if (resourceId || container) void load();
    return () => stopStream();
  });

  $effect(() => {
    // Deep links from other pages replace the current selection.
    const nextResource = router.param('resource');
    if (nextResource && nextResource !== resourceId) {
      resourceId = nextResource;
      container = router.param('container');
    }
  });
</script>

<PageHeader
  title="Logs"
  description="Recent output from one resource or container. Lines are read on demand, redacted best-effort, and never stored."
>
  {#snippet meta()}
    {#if selected}
      <a class="selected" href={resourcePath(selected.id)}>{selected.name}</a>
    {/if}
    {#if range === 'custom' && from && to}
      <Badge tone="info">{formatAbsolute(from)} – {formatAbsolute(to)}</Badge>
    {/if}
  {/snippet}
</PageHeader>

<div class="layout">
  <Card title="Source" padded>
    <div class="filters">
      <Field label="Resource" required>
        {#snippet children({ id })}
          <ResourcePicker
            {id}
            snapshot={live.snapshot}
            bind:value={resourceId}
            onchange={() => (container = '')}
          />
        {/snippet}
      </Field>
      {#if components.length > 1}
        <Field label="Container">
          {#snippet children({ id })}
            <Select {id} bind:value={container}>
              <option value="">All containers</option>
              {#each components as component (component.id)}<option
                  value={component.id}>{component.name}</option
                >{/each}
            </Select>
          {/snippet}
        </Field>
      {/if}
      <div class="range">
        <span class="range-label">Range</span>
        <SegmentedControl
          label="Log range"
          size="sm"
          bind:value={range}
          options={[
            { value: '5m', label: '5m' },
            { value: '30m', label: '30m' },
            { value: '1h', label: '1h' },
            ...(range === 'custom'
              ? [{ value: 'custom' as LogRange, label: 'custom' }]
              : []),
          ]}
        />
      </div>
      <Field label="Search" hint="Literal text match, case-insensitive.">
        {#snippet children({ id, describedBy })}
          <Input
            {id}
            type="search"
            bind:value={search}
            maxlength={256}
            placeholder="error, request id, path…"
            aria-describedby={describedBy}
            onkeydown={(event) => event.key === 'Enter' && void load()}
          >
            {#snippet leading()}<Search />{/snippet}
          </Input>
        {/snippet}
      </Field>
      <div class="switches">
        <Field label="Follow new lines" inline>
          {#snippet children({ id })}<Switch
              {id}
              bind:checked={follow}
            />{/snippet}
        </Field>
        <Field label="Severity colors" inline>
          {#snippet children({ id })}<Switch
              {id}
              bind:checked={colors}
            />{/snippet}
        </Field>
        <Field label="Wrap long lines" inline>
          {#snippet children({ id })}<Switch
              {id}
              bind:checked={wrap}
            />{/snippet}
        </Field>
      </div>
      {#if streaming}
        <Button variant="danger" onclick={stopStream}>
          {#snippet icon()}<Square />{/snippet}
          Stop following
        </Button>
      {:else}
        <Button variant="primary" onclick={load} {loading}>
          {#snippet icon()}<Play />{/snippet}
          {follow ? 'Start following' : 'Load logs'}
        </Button>
      {/if}
      {#if loadedAt}
        <p class="stamp">
          {streaming ? 'Streaming since' : 'Loaded'}
          {formatAbsolute(loadedAt)} · {entries.length}
          {entries.length === 1 ? 'line' : 'lines'}
        </p>
      {/if}
    </div>
  </Card>

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
      {entries}
      following={streaming}
      {wrap}
      {colors}
      {search}
      {loading}
      {componentNames}
    />
    {#snippet footer()}
      <p class="note">
        Logs are not persisted. Redaction is best-effort{redaction
          ? ` (${redaction})`
          : ''}; avoid logging secrets.
      </p>
    {/snippet}
  </Card>
</div>

<style>
  .layout {
    display: grid;
    grid-template-columns: 300px minmax(0, 1fr);
    gap: var(--space-4);
    align-items: start;
  }
  .filters {
    display: grid;
    gap: var(--space-4);
  }
  .range {
    display: grid;
    gap: var(--space-2);
  }
  .range-label {
    font-size: var(--text-sm);
    font-weight: 500;
  }
  .switches {
    display: grid;
    gap: var(--space-2);
  }
  .stamp {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .selected {
    font-family: var(--font-mono);
    font-weight: 500;
  }
  .banner {
    padding: var(--space-2) var(--space-4);
    border-bottom: 1px solid var(--border);
    font-size: var(--text-sm);
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
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  @media (max-width: 1000px) {
    .layout {
      grid-template-columns: 1fr;
    }
  }
</style>
