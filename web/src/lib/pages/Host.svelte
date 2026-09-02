<script lang="ts">
  import { SvelteMap } from 'svelte/reactivity';
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
  import { errorMessage, isApiError } from '../api/client';
  import { bucketize } from '../series';
  import { formatBytes, formatNumber, formatRate } from '../i18n';
  import { formatUptime } from '../watch';
  import { resourcePath } from '../router';
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
  import PageHeader from '../ui/PageHeader.svelte';
  import ProgressBar from '../ui/ProgressBar.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import Stat from '../ui/Stat.svelte';
  import StatusPill from '../ui/StatusPill.svelte';
  import Tabs, { tabPanelId, tabId } from '../ui/Tabs.svelte';
  import { formatClock } from '../ui/relative-time';
  import { utilizationTone } from '../ui/status';
  import { tooltip } from '../ui/tooltip';

  let { live }: { live: LiveStore } = $props();

  let tab = $state('metrics');
  let processes = $state<HostProcess[]>([]);
  let processSort = $state<SortState | undefined>({
    key: 'cpu',
    direction: 'desc',
  });
  let sampledAt = $state<string | null>(null);
  let sampling = $state(false);
  let processError = $state('');
  let processLimit = $state(25);
  let fsSort = $state<SortState | undefined>({
    key: 'used',
    direction: 'desc',
  });

  const snapshot = $derived(live.snapshot);
  const host = $derived(snapshot?.host);
  const now = $derived(snapshot ? new Date(snapshot.ts).getTime() : Date.now());
  const spark = (key: 'cpu' | 'memory' | 'rx' | 'load1') =>
    bucketize(
      liveSeries.hostWindow(key, 3_600_000, now),
      now - 3_600_000,
      now,
      60,
    );
  const memoryPct = $derived(
    host
      ? (host.memoryPct ?? percent(host.memoryUsedBytes, host.memoryTotalBytes))
      : null,
  );
  const filesystems = $derived(snapshot?.filesystems ?? []);
  const collectors = $derived(Object.entries(snapshot?.collectors ?? {}));
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
    { key: 'type', label: 'Type', hideBelow: 900, cell: fsType },
    {
      key: 'used',
      label: 'Used',
      sortable: true,
      sortValue: (row) => row.usedPct,
      cell: fsUsed,
      width: '34%',
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
      hideBelow: 720,
      sortable: true,
      sortValue: (row) => row.totalBytes,
      cell: fsTotal,
    },
    {
      key: 'inodes',
      label: 'Inodes',
      align: 'right',
      hideBelow: 1100,
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
      hideBelow: 900,
      sortable: true,
      sortValue: (row) => row.user,
      cell: processUser,
    },
    {
      key: 'state',
      label: 'State',
      hideBelow: 1100,
      sortable: true,
      sortValue: (row) => row.state,
      cell: processState,
    },
    {
      key: 'uptime',
      label: 'Uptime',
      align: 'right',
      hideBelow: 1000,
      sortable: true,
      sortValue: (row) => row.uptimeSeconds,
      cell: processUptime,
    },
    {
      key: 'container',
      label: 'Container',
      hideBelow: 720,
      cell: processContainer,
    },
  ];

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

  $effect(() => {
    if (tab === 'processes' && !sampledAt && !sampling && !processError)
      void sample();
  });
</script>

{#snippet fsMount(row: FilesystemObservation)}
  <span class="mono strong">{row.mountPoint}</span>
{/snippet}
{#snippet fsType(row: FilesystemObservation)}
  <Badge tone="neutral" mono>{row.fsType || '—'}</Badge>
{/snippet}
{#snippet fsUsed(row: FilesystemObservation)}
  <div class="fs-used">
    <ProgressBar value={row.usedPct} label={`${row.mountPoint} usage`} />
    <span class={`num pct ${utilizationTone(row.usedPct)}`}
      >{row.usedPct == null ? '—' : `${formatNumber(row.usedPct)}%`}</span
    >
  </div>
{/snippet}
{#snippet fsFree(row: FilesystemObservation)}
  {row.availableBytes == null ? '—' : formatBytes(row.availableBytes)}
{/snippet}
{#snippet fsTotal(row: FilesystemObservation)}
  {row.totalBytes == null ? '—' : formatBytes(row.totalBytes)}
{/snippet}
{#snippet fsInodes(row: FilesystemObservation)}
  <span class={utilizationTone(row.inodesUsedPct)}
    >{row.inodesUsedPct == null
      ? '—'
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
  <span class="mono command" use:tooltip={row.command}>{row.command}</span>
{/snippet}
{#snippet processCpu(row: HostProcess)}
  {row.cpuPct == null ? '—' : `${formatNumber(row.cpuPct)}%`}
{/snippet}
{#snippet processRss(row: HostProcess)}
  {row.rssBytes == null ? '—' : formatBytes(row.rssBytes)}
{/snippet}
{#snippet processUser(row: HostProcess)}
  <span class="mono">{row.user || row.uid || '—'}</span>
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

<PageHeader
  title="Host"
  description="The Linux server itself: CPU, memory, disks, network, and the collectors that observe it."
>
  {#snippet meta()}
    {#if snapshot}
      <span class="meta-item"
        >Uptime <span class="num">{formatUptime(host?.uptimeSeconds)}</span
        ></span
      >
      <span class="meta-item"
        >Boot <span class="num">{snapshot.bootIdentity || '—'}</span></span
      >
      {#each collectors as [name, collector] (name)}
        <StatusPill
          status={collector.state}
          label={`${name} collector`}
          size="sm"
        />
      {/each}
    {/if}
  {/snippet}
  {#if snapshot && host}
    <div class="stats">
      <Stat
        label="CPU"
        value={host.cpuPct == null ? '—' : formatNumber(host.cpuPct)}
        unit={host.cpuPct == null ? '' : '%'}
        percent={host.cpuPct ?? null}
        sparkline={spark('cpu')}
        sparklineMax={100}
        sparklineLabel="CPU over the last hour"
        secondary={host.load1 == null
          ? ''
          : `load ${formatNumber(host.load1)} · ${formatNumber(host.load5)} · ${formatNumber(host.load15)}`}
      />
      <Stat
        label="Memory"
        value={host.memoryUsedBytes == null
          ? '—'
          : formatBytes(host.memoryUsedBytes)}
        percent={memoryPct}
        sparkline={spark('memory')}
        sparklineMax={100}
        sparklineLabel="Memory over the last hour"
        secondary={host.memoryTotalBytes == null
          ? ''
          : `of ${formatBytes(host.memoryTotalBytes)}${host.swapUsedBytes != null ? ` · swap ${formatBytes(host.swapUsedBytes)}` : ''}`}
      />
      <Stat
        label="Network"
        value={host.networkRxBps == null ? '—' : formatRate(host.networkRxBps)}
        secondary={host.networkTxBps == null
          ? ''
          : `↓ receive · ↑ ${formatRate(host.networkTxBps)}`}
        sparkline={spark('rx')}
        sparklineLabel="Receive rate over the last hour"
        tone="accent"
      />
      <Stat
        label="Disk I/O"
        value={host.diskReadBps == null ? '—' : formatRate(host.diskReadBps)}
        secondary={host.diskWriteBps == null
          ? ''
          : `read · write ${formatRate(host.diskWriteBps)}${host.diskReadIops != null ? ` · ${formatNumber((host.diskReadIops ?? 0) + (host.diskWriteIops ?? 0))} IOPS` : ''}`}
        tone="accent"
      />
    </div>
  {/if}
  <Tabs
    prefix="host"
    label="Host sections"
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

{#if !snapshot}
  <div
    class="loading"
    role="status"
    aria-label="Waiting for the first live sample"
  >
    <Skeleton height={110} />
    <Skeleton height={320} />
  </div>
{:else}
  <div
    id={tabPanelId('host', 'metrics')}
    role="tabpanel"
    aria-labelledby={tabId('host', 'metrics')}
    hidden={tab !== 'metrics'}
  >
    {#if tab === 'metrics'}
      <MetricCharts scope="host" groups={chartGroups} />
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
            Filesystem alerts warn at 80% and turn critical at 95% by default.
            Adjust thresholds under Alerts → Rules.
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
            Two /proc samples a moment apart, on demand only. Binnacle never
            signals, renices, or inspects processes beyond this list.
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
      <div class="collectors">
        {#each collectors as [name, collector] (name)}
          <Card title={`${name} collector`}>
            {#snippet actions()}<StatusPill
                status={collector.state}
              />{/snippet}
            <dl class="collector-meta">
              <div>
                <dt>Fresh at</dt>
                <dd>
                  {collector.freshAt
                    ? new Date(collector.freshAt).toLocaleString()
                    : '—'}
                </dd>
              </div>
              <div>
                <dt>Reason</dt>
                <dd>{collector.reason ?? 'Operating normally'}</dd>
              </div>
              <div>
                <dt>Observes</dt>
                <dd>
                  {name === 'host'
                    ? 'CPU, memory, load, filesystems, network, and disk I/O from /proc and /sys.'
                    : name === 'docker'
                      ? 'Containers, their stats, and lifecycle events from the Docker API.'
                      : name === 'coolify'
                        ? 'Project and environment metadata from the Coolify API.'
                        : '—'}
                </dd>
              </div>
            </dl>
          </Card>
        {/each}
      </div>
    {/if}
  </div>
{/if}

<style>
  .meta-item {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .meta-item .num {
    color: var(--text-2);
  }
  .stats {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    overflow: hidden;
  }
  .stats > :global(.stat) {
    border-right: 1px solid var(--border);
  }
  .stats > :global(.stat:last-child) {
    border-right: 0;
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
    color: var(--text-3);
  }
  .fs-used {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 160px;
  }
  .pct {
    min-width: 5ch;
    text-align: right;
  }
  .pct.warn,
  :global(.warn).pct {
    color: var(--warn-fg);
  }
  .pct.critical {
    color: var(--critical-fg);
  }
  .command {
    display: inline-block;
    max-width: 420px;
    overflow: hidden;
    text-overflow: ellipsis;
    vertical-align: middle;
  }
  .process-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    min-width: 0;
  }
  .sampled {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .limit {
    height: var(--control-h-sm);
    padding: 0 var(--space-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    color: var(--text);
    font-size: var(--text-xs);
  }
  .error {
    padding: var(--space-3) var(--space-4);
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  .note {
    margin-right: auto;
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .collectors {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
    gap: var(--space-4);
  }
  .collector-meta {
    display: grid;
    gap: var(--space-3);
    margin: 0;
    font-size: var(--text-sm);
  }
  .collector-meta div {
    display: grid;
    gap: 2px;
  }
  .collector-meta dt {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .collector-meta dd {
    margin: 0;
  }
  @media (max-width: 1000px) {
    .stats {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .stats > :global(.stat:nth-child(2)) {
      border-right: 0;
    }
    .stats > :global(.stat:nth-child(-n + 2)) {
      border-bottom: 1px solid var(--border);
    }
  }
</style>
