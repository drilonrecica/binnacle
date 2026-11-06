<script lang="ts">
  import { onMount } from 'svelte';
  import { SvelteMap } from 'svelte/reactivity';
  import Copy from '@lucide/svelte/icons/copy';
  import HardDrive from '@lucide/svelte/icons/hard-drive';
  import ListTree from '@lucide/svelte/icons/list-tree';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import type { FilesystemObservation, LiveStore } from '../live.svelte';
  import { liveSeries, percent } from '../live-series.svelte';
  import {
    processStateLabel,
    sampleProcesses,
    type HostProcess,
  } from '../api/processes';
  import { listRules, type Rule } from '../api/alerts';
  import { errorMessage, isApiError } from '../api/client';
  import { formatBytes, formatNumber } from '../i18n';
  import { formatUptime } from '../watch';
  import { resourcePath } from '../router';
  import { router } from '../router.svelte';
  import { connectionView } from '../shell/connection';
  import { gaugeThresholds, diskLabel } from '../overview/headroom';
  import Instruments from '../overview/Instruments.svelte';
  import {
    chartFocus,
    collectorViews,
    commandParts,
    filesystemTrend,
    missing,
    serverFilesystems,
    serverInstruments,
  } from '../server/server';
  import type { ChartGroup } from '../charts/MetricCharts.svelte';
  import MetricCharts from '../charts/MetricCharts.svelte';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import DataTable, {
    type Column,
    type SortState,
  } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import ErrorState from '../ui/ErrorState.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import Scale from '../ui/Scale.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import StaleBanner from '../ui/StaleBanner.svelte';
  import Status from '../ui/Status.svelte';
  import Tabs, { tabPanelId, tabId } from '../ui/Tabs.svelte';
  import { formatAbsolute, formatClock } from '../ui/relative-time';
  import { utilizationTone } from '../ui/status';
  import { toasts } from '../ui/toast.svelte';
  import { tooltip } from '../ui/tooltip';

  let { live }: { live: LiveStore } = $props();

  const hourMs = 3_600_000;

  let tab = $state('metrics');
  let rules = $state<Rule[]>([]);
  let processes = $state<HostProcess[]>([]);
  let processSort = $state<SortState | undefined>({
    key: 'cpu',
    direction: 'desc',
  });
  let sampledAt = $state<string | null>(null);
  let sampling = $state(false);
  let processError = $state('');
  let processLimit = $state(25);
  let fsSort = $state<SortState | undefined>(undefined);
  // Decides when quiet data turns stale; nothing it drives is announced.
  let now = $state(Date.now());

  const snapshot = $derived(live.snapshot);
  const host = $derived(snapshot?.host);
  const connection = $derived(
    connectionView(live.state, live.lastReceivedAt, now),
  );
  const streamLive = $derived(connection.tone === 'ok');
  const failedToStart = $derived(
    !snapshot &&
      (live.state === 'disconnected' || live.state === 'unauthorized'),
  );
  const thresholds = $derived(gaugeThresholds(rules));
  const instruments = $derived(
    snapshot
      ? serverInstruments(
          snapshot,
          thresholds,
          liveSeries.hostWindow('cpu', hourMs, Date.parse(snapshot.ts)),
        )
      : [],
  );
  const filesystems = $derived(snapshot ? serverFilesystems(snapshot) : []);
  const collectors = $derived(collectorViews(snapshot?.collectors));
  const troubled = $derived(
    collectors.filter((collector) => collector.state !== 'healthy'),
  );
  // Links from Headroom: `?metric=cpu` focuses a chart, `?mount=` a disk.
  const focusChart = $derived(chartFocus(router.param('metric')));
  const focusMount = $derived(router.param('mount'));
  const containerNames = $derived.by(() => {
    const map = new SvelteMap<string, { resourceId: string; name: string }>();
    for (const resource of snapshot?.resources ?? []) {
      for (const component of resource.components ?? []) {
        map.set(component.id.slice(0, 12), {
          resourceId: resource.id,
          name: component.name,
        });
      }
    }
    return map;
  });

  const chartGroups: ChartGroup[] = [
    {
      key: 'cpu',
      title: 'CPU',
      description: 'Busy time by category',
      metrics: [
        { metric: 'cpu', label: 'Busy', color: 1 },
        { metric: 'cpu_user', label: 'User', color: 2 },
        { metric: 'cpu_system', label: 'System', color: 4 },
        { metric: 'cpu_iowait', label: 'I/O wait', color: 3 },
        { metric: 'cpu_steal', label: 'Steal', color: 6 },
      ],
      format: 'percent',
      max: 100,
    },
    {
      key: 'load',
      title: 'Load average',
      metrics: [
        { metric: 'load_1', label: '1 min', color: 1 },
        { metric: 'load_5', label: '5 min', color: 5 },
        { metric: 'load_15', label: '15 min', color: 4 },
      ],
      format: 'load',
    },
    {
      key: 'memory',
      title: 'Memory',
      metrics: [
        { metric: 'memory', label: 'Used', color: 5 },
        { metric: 'swap', label: 'Swap', color: 3 },
      ],
      format: 'bytes',
    },
    {
      key: 'network',
      title: 'Network',
      metrics: [
        { metric: 'network_rx', label: 'Receive', color: 1 },
        { metric: 'network_tx', label: 'Transmit', color: 4 },
      ],
      format: 'rate',
    },
    {
      key: 'disk',
      title: 'Disk throughput',
      metrics: [
        { metric: 'disk_read', label: 'Read', color: 2 },
        { metric: 'disk_write', label: 'Write', color: 3 },
      ],
      format: 'rate',
    },
    {
      key: 'iops',
      title: 'Disk operations',
      metrics: [{ metric: 'disk_iops', label: 'IOPS', color: 6 }],
      format: 'iops',
    },
  ];

  const fsColumns: Column<FilesystemObservation>[] = [
    {
      key: 'mount',
      label: 'Mount',
      sortable: true,
      sortValue: (row) => row.mountPoint,
      cell: fsMount,
    },
    { key: 'type', label: 'Type', hideBelow: 'lg', cell: fsType },
    {
      key: 'used',
      label: 'Used',
      sortable: true,
      sortValue: (row) => usedPct(row),
      cell: fsUsed,
      width: '30%',
    },
    {
      key: 'full',
      label: 'Time to full',
      sortable: true,
      sortValue: (row) =>
        row.trend?.state === 'filling' ? (row.trend.daysToFull ?? null) : null,
      cell: fsFull,
    },
    {
      key: 'free',
      label: 'Free',
      align: 'right',
      sortable: true,
      sortValue: (row) => row.availableBytes,
      cell: fsFree,
    },
    {
      key: 'total',
      label: 'Total',
      align: 'right',
      sortable: true,
      sortValue: (row) => row.totalBytes,
      cell: fsTotal,
    },
    {
      key: 'inodes',
      label: 'Inodes',
      align: 'right',
      hideBelow: 'lg',
      sortable: true,
      sortValue: (row) => row.inodesUsedPct,
      cell: fsInodes,
    },
  ];

  const processColumns: Column<HostProcess>[] = [
    {
      key: 'pid',
      label: 'PID',
      width: '80px',
      sortable: true,
      sortValue: (row) => row.pid,
      cell: processPid,
    },
    {
      key: 'command',
      label: 'Command',
      card: 'title',
      sortable: true,
      sortValue: (row) => row.command,
      cell: processCommand,
    },
    {
      key: 'cpu',
      label: 'CPU',
      align: 'right',
      width: '90px',
      sortable: true,
      sortValue: (row) => row.cpuPct,
      cell: processCpu,
    },
    {
      key: 'rss',
      label: 'Memory',
      align: 'right',
      width: '110px',
      sortable: true,
      sortValue: (row) => row.rssBytes,
      cell: processRss,
    },
    {
      key: 'user',
      label: 'User',
      sortable: true,
      sortValue: (row) => row.user,
      cell: processUser,
    },
    {
      key: 'state',
      label: 'State',
      hideBelow: 'lg',
      sortable: true,
      sortValue: (row) => row.state,
      cell: processState,
    },
    {
      key: 'uptime',
      label: 'Uptime',
      align: 'right',
      hideBelow: 'lg',
      sortable: true,
      sortValue: (row) => row.uptimeSeconds,
      cell: processUptime,
    },
    {
      key: 'container',
      label: 'Container',
      cell: processContainer,
    },
  ];

  function usedPct(row: FilesystemObservation) {
    return row.usedPct ?? percent(row.usedBytes, row.totalBytes);
  }

  async function copyBootId(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      toasts.success('Boot id copied');
    } catch {
      toasts.error('Could not copy the boot id');
    }
  }

  async function sample() {
    if (sampling) return;
    sampling = true;
    processError = '';
    try {
      const result = await sampleProcesses(processLimit);
      processes = result.processes;
      sampledAt = new Date().toISOString();
    } catch (reason) {
      if (isApiError(reason, 'scan_busy'))
        processError =
          'Another process scan is running. Try again in a moment.';
      else processError = errorMessage(reason);
    } finally {
      sampling = false;
    }
  }

  async function loadRules() {
    try {
      rules = await listRules();
    } catch {
      // Gauges fall back to the design thresholds.
      rules = [];
    }
  }

  $effect(() => {
    if (tab === 'processes' && !sampledAt && !sampling && !processError)
      void sample();
  });

  onMount(() => {
    void loadRules();
    const clock = window.setInterval(() => (now = Date.now()), 2_000);
    return () => window.clearInterval(clock);
  });
</script>

{#snippet fsMount(row: FilesystemObservation)}
  <span class="mount" class:focused={row.mountKey === focusMount}>
    <span class="mono strong">{row.mountPoint}</span>
    <span class="role">{diskLabel(row)}</span>
  </span>
{/snippet}
{#snippet fsType(row: FilesystemObservation)}
  <Badge tone="neutral" mono>{row.fsType || missing}</Badge>
{/snippet}
{#snippet fsUsed(row: FilesystemObservation)}
  {@const limits = thresholds.filesystem(row.mountKey)}
  {@const pct = usedPct(row)}
  <div class="fs-used">
    <Scale
      value={pct}
      label={`${row.mountPoint} usage`}
      threshold={limits.critical}
    />
    <span
      class={`num pct ${utilizationTone(pct, limits.warn, limits.critical)}`}
      >{pct == null ? missing : `${formatNumber(pct)}%`}</span
    >
  </div>
{/snippet}
{#snippet fsFull(row: FilesystemObservation)}
  {@const trend = filesystemTrend(row, thresholds)}
  <span class={`trend ${trend.tone}`}>{trend.text}</span>
{/snippet}
{#snippet fsFree(row: FilesystemObservation)}
  {row.availableBytes == null ? missing : formatBytes(row.availableBytes)}
{/snippet}
{#snippet fsTotal(row: FilesystemObservation)}
  {row.totalBytes == null ? missing : formatBytes(row.totalBytes)}
{/snippet}
{#snippet fsInodes(row: FilesystemObservation)}
  <span class={utilizationTone(row.inodesUsedPct)}
    >{row.inodesUsedPct == null
      ? missing
      : `${formatNumber(row.inodesUsedPct)}%`}</span
  >
{/snippet}
{#snippet fsEmpty()}
  <EmptyState
    title="No filesystem data yet"
    description="Per-mount usage appears once the host collector reads mounted filesystems."
  >
    {#snippet icon()}<HardDrive />{/snippet}
  </EmptyState>
{/snippet}

{#snippet processPid(row: HostProcess)}
  <span class="num">{row.pid}</span>
{/snippet}
{#snippet processCommand(row: HostProcess)}
  <span class="mono command" use:tooltip={row.command}
    >{#each commandParts(row.command) as part, index (index)}{#if part.redacted}<span
          class="redacted">{part.text}</span
        >{:else}{part.text}{/if}{/each}</span
  >
{/snippet}
{#snippet processCpu(row: HostProcess)}
  {row.cpuPct == null ? missing : `${formatNumber(row.cpuPct)}%`}
{/snippet}
{#snippet processRss(row: HostProcess)}
  {row.rssBytes == null ? missing : formatBytes(row.rssBytes)}
{/snippet}
{#snippet processUser(row: HostProcess)}
  <span class="mono">{row.user || row.uid || missing}</span>
{/snippet}
{#snippet processState(row: HostProcess)}
  {processStateLabel(row.state)}
{/snippet}
{#snippet processUptime(row: HostProcess)}
  {formatUptime(row.uptimeSeconds)}
{/snippet}
{#snippet processContainer(row: HostProcess)}
  {#if row.containerId}
    {@const known = containerNames.get(row.containerId.slice(0, 12))}
    {#if known}
      <a href={resourcePath(known.resourceId)} class="mono">{known.name}</a>
    {:else}
      <code class="mono">{row.containerId.slice(0, 12)}</code>
    {/if}
  {:else}
    <span class="muted">host</span>
  {/if}
{/snippet}
{#snippet processEmpty()}
  <EmptyState
    title="No process sample yet"
    description="Sampling reads /proc once and is never persisted."
  >
    {#snippet icon()}<ListTree />{/snippet}
    <Button variant="primary" onclick={sample} loading={sampling}
      >Sample now</Button
    >
  </EmptyState>
{/snippet}

<PageHeader title="Server">
  {#snippet meta()}
    {#if snapshot}
      <span class="meta-item"
        >up <span class="num">{formatUptime(host?.uptimeSeconds)}</span></span
      >
      {#if snapshot.bootIdentity}
        <button
          type="button"
          class="id"
          onclick={() => copyBootId(snapshot.bootIdentity)}
          aria-label={`boot ${snapshot.bootIdentity}, copy the boot id`}
          use:tooltip={'Copy boot id'}
        >
          <span class="id-label">boot</span>
          <code>{snapshot.bootIdentity}</code><Copy aria-hidden="true" />
        </button>
      {/if}
      {#each troubled as collector (collector.name)}
        <Status
          status={collector.state}
          label={`${collector.title} ${collector.word.toLowerCase()}`}
          size="sm"
        />
      {/each}
    {/if}
  {/snippet}
  <Tabs
    prefix="host"
    label="Server sections"
    param="tab"
    bind:active={tab}
    tabs={[
      { id: 'metrics', label: 'Metrics' },
      {
        id: 'filesystems',
        label: 'Filesystems',
        count: filesystems.length || null,
      },
      { id: 'processes', label: 'Processes' },
      {
        id: 'collectors',
        label: 'Collectors',
        count: collectors.length || null,
      },
    ]}
  />
</PageHeader>

{#if failedToStart}
  <ErrorState
    title="Live data could not be loaded"
    description={live.error ||
      'The live stream did not start. A proxy in front of Binnacle may be buffering it.'}
    onretry={() => live.retry()}
    retryLabel="Retry now"
  />
{:else if !snapshot}
  <div
    class="loading"
    role="status"
    aria-label="Waiting for the first live sample"
  >
    <Skeleton height={150} />
    <Skeleton height={320} />
  </div>
{:else}
  {#if !streamLive}
    <div class="stale">
      <StaleBanner
        message="Showing the last known values"
        since={live.lastReceivedAt}
        tone={live.state === 'disconnected' ? 'critical' : 'warn'}
      >
        {#if live.state === 'disconnected'}
          <Button size="sm" onclick={() => live.retry()}>Retry now</Button>
        {/if}
      </StaleBanner>
    </div>
  {/if}

  <Instruments
    set={{ instruments, moreDisks: 0 }}
    stale={!streamLive}
    heading="Server instruments"
    headingId="server-instruments-title"
  />

  <div
    id={tabPanelId('host', 'metrics')}
    role="tabpanel"
    aria-labelledby={tabId('host', 'metrics')}
    hidden={tab !== 'metrics'}
  >
    {#if tab === 'metrics'}
      <MetricCharts scope="host" groups={chartGroups} focus={focusChart} />
    {/if}
  </div>

  <div
    id={tabPanelId('host', 'filesystems')}
    role="tabpanel"
    aria-labelledby={tabId('host', 'filesystems')}
    hidden={tab !== 'filesystems'}
  >
    {#if tab === 'filesystems'}
      <Card padded={false}>
        <DataTable
          rows={filesystems}
          columns={fsColumns}
          rowKey={(row) => row.mountKey}
          caption="Filesystems"
          bind:sort={fsSort}
          empty={fsEmpty}
        />
        {#snippet footer()}
          <p class="note">
            Thresholds and the "full within" warning come from the filesystem
            alert rules in Settings › Alerting. Time to full needs three hours
            of history.
          </p>
        {/snippet}
      </Card>
    {/if}
  </div>

  <div
    id={tabPanelId('host', 'processes')}
    role="tabpanel"
    aria-labelledby={tabId('host', 'processes')}
    hidden={tab !== 'processes'}
  >
    {#if tab === 'processes'}
      <Card padded={false}>
        {#snippet actions()}
          <div class="process-actions">
            {#if sampledAt}<span class="sampled"
                >Sampled at <span class="num">{formatClock(sampledAt)}</span> · <RelativeTime
                  value={sampledAt}
                /></span
              >{/if}
            <select
              class="limit"
              bind:value={processLimit}
              aria-label="Rows to sample"
            >
              <option value={25}>Top 25</option>
              <option value={50}>Top 50</option>
              <option value={100}>Top 100</option>
            </select>
            <Button size="sm" onclick={sample} loading={sampling}>
              {#snippet icon()}<RefreshCw />{/snippet}
              Sample now
            </Button>
          </div>
        {/snippet}
        {#if processError}
          <p class="error" role="alert">{processError}</p>
        {/if}
        <DataTable
          rows={processes}
          columns={processColumns}
          rowKey={(row) => String(row.pid)}
          caption="Host processes"
          bind:sort={processSort}
          loading={sampling && !processes.length}
          empty={processEmpty}
          density="compact"
        />
        {#snippet footer()}
          <p class="note">
            Two /proc samples a moment apart, on demand only, never stored.
            Command lines are redacted on the server: passwords, tokens and
            other secret arguments show as <span class="redacted"
              >[REDACTED]</span
            >. Binnacle never signals, renices, or inspects processes beyond
            this list.
          </p>
        {/snippet}
      </Card>
    {/if}
  </div>

  <div
    id={tabPanelId('host', 'collectors')}
    role="tabpanel"
    aria-labelledby={tabId('host', 'collectors')}
    hidden={tab !== 'collectors'}
  >
    {#if tab === 'collectors'}
      {#if collectors.length}
        <div class="collectors">
          {#each collectors as collector (collector.name)}
            <Card title={collector.title}>
              {#snippet actions()}<Status
                  status={collector.state}
                  label={collector.word}
                />{/snippet}
              <dl class="collector-meta">
                <div>
                  <dt>State</dt>
                  <dd>{collector.word}</dd>
                </div>
                <div>
                  <dt>Reason</dt>
                  <dd>{collector.reason}</dd>
                </div>
                <div>
                  <dt>Last fresh data</dt>
                  <dd>
                    {#if collector.freshAt}
                      <span class="num"
                        >{formatAbsolute(collector.freshAt)}</span
                      >
                      · <RelativeTime value={collector.freshAt} />
                    {:else}
                      {missing}
                    {/if}
                  </dd>
                </div>
                <div>
                  <dt>Observes</dt>
                  <dd>{collector.observes}</dd>
                </div>
              </dl>
            </Card>
          {/each}
        </div>
      {:else}
        <Card>
          <EmptyState
            title="No collector status yet"
            description="Collector states appear with the first live sample that reports them."
          />
        </Card>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .meta-item {
    color: var(--muted);
    font-size: var(--text-mono);
  }
  .meta-item .num {
    color: var(--muted);
  }
  .id {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    max-width: 100%;
    padding: 2px var(--space-2);
    border: 1px solid var(--line);
    border-radius: var(--radius-control);
    background: var(--panel);
    color: var(--muted);
    font-size: var(--text-mono);
    cursor: pointer;
  }
  .id:hover {
    color: var(--fg);
  }
  .id code {
    overflow: hidden;
    font-family: var(--font-mono);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .id :global(svg) {
    flex: none;
    width: 12px;
    height: 12px;
  }
  .id-label {
    flex: none;
  }
  .stale {
    margin-bottom: var(--space-4);
  }
  .loading {
    display: grid;
    gap: var(--space-4);
  }
  .mono {
    font-family: var(--font-mono);
  }
  .strong {
    font-weight: 600;
  }
  .muted {
    color: var(--muted);
  }
  .mount {
    display: inline-grid;
    gap: 2px;
    padding-left: var(--space-2);
    border-left: 2px solid transparent;
  }
  .mount.focused {
    border-left-color: var(--accent);
  }
  .role {
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .fs-used {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 140px;
  }
  .pct {
    min-width: 5ch;
    text-align: right;
  }
  .pct.warn {
    color: var(--warn-fg);
  }
  .pct.critical {
    color: var(--critical-fg);
  }
  .trend {
    font-size: var(--text-meta);
  }
  .trend.accent {
    color: var(--accent-text);
  }
  .trend.critical {
    color: var(--critical-fg);
  }
  .trend.muted {
    color: var(--muted);
  }
  .command {
    display: inline-block;
    max-width: 420px;
    overflow: hidden;
    text-overflow: ellipsis;
    vertical-align: middle;
    white-space: nowrap;
  }
  .redacted {
    color: var(--muted);
    font-family: var(--font-mono);
    font-style: italic;
  }
  .process-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    min-width: 0;
  }
  .sampled {
    color: var(--muted);
    font-size: var(--text-mono);
  }
  .limit {
    height: var(--control-h-sm);
    padding: 0 var(--space-2);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-control);
    background: var(--bg);
    color: var(--fg);
    font-size: var(--text-mono);
  }
  .error {
    padding: var(--space-3) var(--space-4);
    color: var(--critical-fg);
    font-size: var(--text-meta);
  }
  .note {
    margin-right: auto;
    color: var(--muted);
    font-size: var(--text-mono);
  }
  .collectors {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
    gap: var(--space-4);
  }
  .collector-meta {
    display: grid;
    gap: var(--space-3);
    margin: 0;
    font-size: var(--text-meta);
  }
  .collector-meta div {
    display: grid;
    gap: 2px;
  }
  .collector-meta dt {
    color: var(--muted);
    font-size: var(--text-mono);
  }
  .collector-meta dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
</style>
