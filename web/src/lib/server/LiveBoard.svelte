<script lang="ts">
  import { onMount } from 'svelte';
  import { SvelteMap } from 'svelte/reactivity';
  import ListTree from '@lucide/svelte/icons/list-tree';
  import type {
    DiskDeviceRate,
    FilesystemObservation,
    InterfaceRate,
    LiveSnapshot,
  } from '../live.svelte';
  import { percent } from '../live-series.svelte';
  import {
    processStateLabel,
    sampleProcesses,
    type HostProcess,
    type ProcessSample,
    type TaskCounts,
  } from '../api/processes';
  import {
    fetchContainerDetails,
    fetchServerIdentity,
    type ContainerDetail,
    type HostIdentity,
  } from '../api/server';
  import { errorMessage, isApiError } from '../api/client';
  import { formatBytes, formatNumber, missing } from '../i18n';
  import { resourcePath } from '../router';
  import { formatUptime } from '../watch';
  import { commandParts } from './server';
  import {
    activityReadings,
    containerRows,
    cpuReadings,
    formatCpuTime,
    loadReadings,
    memoryOfLimit,
    memoryReadings,
    rwRates,
    swapReadings,
    type ContainerRow,
    type Reading,
  } from './live-board';
  import { startPoller } from './poller';
  import Card from '../ui/Card.svelte';
  import DataTable, {
    type Column,
    type SortState,
  } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Status from '../ui/Status.svelte';
  import { formatClock } from '../ui/relative-time';
  import { utilizationTone } from '../ui/status';
  import { tooltip } from '../ui/tooltip';

  /** The live snapshot; `now` is the page's 2 s clock for uptimes. */
  let { snapshot, now }: { snapshot: LiveSnapshot; now: number } = $props();

  /** Processes refresh this often while the tab is open and visible. */
  const processIntervalMs = 5_000;
  const processLimit = 25;
  /** Image, ports, start time and limit change rarely; refetched this often. */
  const detailIntervalMs = 30_000;

  let identity = $state.raw<HostIdentity>({});
  let details = $state.raw<ReadonlyMap<string, ContainerDetail>>(new Map());
  let processes = $state.raw<HostProcess[]>([]);
  let tasks = $state.raw<TaskCounts | null>(null);
  let sampledAt = $state<string | null>(null);
  let processError = $state('');
  let containerSort = $state<SortState | undefined>({
    key: 'cpu',
    direction: 'desc',
  });
  let processSort = $state<SortState | undefined>({
    key: 'cpu',
    direction: 'desc',
  });

  const host = $derived(snapshot.host);
  const cores = $derived(host.cpuCores ?? []);
  const coreCount = $derived(identity.cpuCores || cores.length || undefined);
  const containers = $derived(containerRows(snapshot, now, details));
  const running = $derived(
    containers.filter((row) => row.runtimeState === 'running').length,
  );
  const interfaces = $derived(host.interfaces ?? []);
  const disks = $derived(host.disks ?? []);
  const filesystems = $derived(snapshot.filesystems ?? []);
  const containerNames = $derived.by(() => {
    const map = new SvelteMap<string, { resourceId: string; name: string }>();
    for (const row of containers)
      map.set(row.id.slice(0, 12), {
        resourceId: row.resourceId,
        name: row.name,
      });
    return map;
  });
  const identityLine = $derived(
    [
      identity.os,
      identity.kernel && `Linux ${identity.kernel}`,
      identity.cpuModel &&
        (identity.cpuCores
          ? `${identity.cpuModel} × ${identity.cpuCores}`
          : identity.cpuModel),
    ].filter(Boolean) as string[],
  );

  function pct(value: number | null | undefined) {
    return value == null ? missing : `${formatNumber(value)}%`;
  }
  function rate(value: number | null | undefined) {
    return value == null ? missing : `${formatBytes(value)}/s`;
  }
  function iops(value: number | null | undefined) {
    return value == null ? missing : `${formatNumber(Math.round(value))}/s`;
  }

  const interfaceColumns: Column<InterfaceRate>[] = [
    { key: 'name', label: 'Interface', cell: ifaceName, card: 'title' },
    {
      key: 'rx',
      label: 'Receive',
      align: 'right',
      sortable: true,
      sortValue: (row) => row.rxBps,
      cell: ifaceRx,
    },
    {
      key: 'tx',
      label: 'Send',
      align: 'right',
      sortable: true,
      sortValue: (row) => row.txBps,
      cell: ifaceTx,
    },
  ];

  const diskColumns: Column<DiskDeviceRate>[] = [
    { key: 'name', label: 'Device', cell: diskName, card: 'title' },
    {
      key: 'read',
      label: 'Read',
      align: 'right',
      sortValue: (row) => row.readBps,
      cell: diskRead,
    },
    {
      key: 'write',
      label: 'Write',
      align: 'right',
      sortValue: (row) => row.writeBps,
      cell: diskWrite,
    },
    {
      key: 'ops',
      label: 'Ops r · w',
      align: 'right',
      hideBelow: 'lg',
      cell: diskOps,
    },
  ];

  const fsColumns: Column<FilesystemObservation>[] = [
    { key: 'mount', label: 'Mount', cell: fsMount, card: 'title' },
    {
      key: 'used',
      label: 'Used',
      align: 'right',
      sortValue: (row) => row.usedPct,
      cell: fsUsed,
    },
    { key: 'total', label: 'Total', align: 'right', cell: fsTotal },
  ];

  const containerColumns: Column<ContainerRow>[] = [
    {
      key: 'name',
      label: 'Container',
      card: 'title',
      sortable: true,
      sortValue: (row) => row.name,
      cell: containerName,
    },
    {
      key: 'status',
      label: 'Status',
      width: '120px',
      sortable: true,
      sortValue: (row) => row.status,
      cell: containerStatus,
    },
    {
      key: 'uptime',
      label: 'Uptime',
      align: 'right',
      sortable: true,
      sortValue: (row) => row.uptimeSeconds,
      cell: containerUptime,
    },
    {
      key: 'cpu',
      label: 'CPU',
      align: 'right',
      sortable: true,
      sortValue: (row) => row.cpuHostPct,
      cell: containerCpu,
    },
    {
      key: 'memory',
      label: 'Memory',
      align: 'right',
      sortable: true,
      sortValue: (row) => row.memoryBytes,
      cell: containerMemory,
    },
    {
      key: 'memoryPct',
      label: 'Mem %',
      align: 'right',
      sortable: true,
      sortValue: (row) => row.memoryPct,
      cell: containerMemoryPct,
    },
    {
      key: 'block',
      label: 'Disk r · w /s',
      align: 'right',
      hideBelow: 'lg',
      sortValue: (row) => (row.blockReadBps ?? 0) + (row.blockWriteBps ?? 0),
      sortable: true,
      cell: containerBlock,
    },
    {
      key: 'net',
      label: 'Net rx · tx /s',
      align: 'right',
      hideBelow: 'lg',
      sortValue: (row) => (row.rxBps ?? 0) + (row.txBps ?? 0),
      sortable: true,
      cell: containerNet,
    },
    {
      key: 'pids',
      label: 'PIDs',
      align: 'right',
      hideBelow: 'lg',
      sortable: true,
      sortValue: (row) => row.pids,
      cell: containerPids,
    },
    {
      key: 'ports',
      label: 'Ports',
      hideBelow: 'lg',
      cell: containerPorts,
    },
  ];

  const processColumns: Column<HostProcess>[] = [
    {
      key: 'cpu',
      label: 'CPU',
      align: 'right',
      width: '72px',
      sortable: true,
      sortValue: (row) => row.cpuPct,
      cell: processCpu,
    },
    {
      key: 'mem',
      label: 'Mem',
      align: 'right',
      width: '72px',
      sortable: true,
      sortValue: (row) => row.memPct,
      cell: processMem,
    },
    {
      key: 'virt',
      label: 'Virt',
      align: 'right',
      hideBelow: 'lg',
      sortable: true,
      sortValue: (row) => row.virtBytes,
      cell: processVirt,
    },
    {
      key: 'res',
      label: 'Res',
      align: 'right',
      sortable: true,
      sortValue: (row) => row.rssBytes,
      cell: processRes,
    },
    {
      key: 'pid',
      label: 'PID',
      align: 'right',
      sortable: true,
      sortValue: (row) => row.pid,
      cell: processPid,
    },
    {
      key: 'user',
      label: 'User',
      sortable: true,
      sortValue: (row) => row.user,
      cell: processUser,
    },
    {
      key: 'time',
      label: 'Time',
      align: 'right',
      hideBelow: 'lg',
      sortable: true,
      sortValue: (row) => row.cpuTimeSeconds,
      cell: processTime,
    },
    {
      key: 'threads',
      label: 'Thr',
      align: 'right',
      hideBelow: 'lg',
      sortable: true,
      sortValue: (row) => row.threads,
      cell: processThreads,
    },
    {
      key: 'nice',
      label: 'Ni',
      align: 'right',
      hideBelow: 'lg',
      sortable: true,
      sortValue: (row) => row.nice,
      cell: processNice,
    },
    {
      key: 'state',
      label: 'S',
      hideBelow: 'lg',
      sortable: true,
      sortValue: (row) => row.state,
      cell: processState,
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
      key: 'container',
      label: 'Container',
      hideBelow: 'lg',
      cell: processContainer,
    },
  ];

  function applySample(result: ProcessSample) {
    processes = result.processes;
    tasks = result.tasks ?? null;
    sampledAt = result.sampledAt ?? new Date().toISOString();
    processError = '';
  }

  onMount(() => {
    const controller = new AbortController();
    fetchServerIdentity(controller.signal).then(
      (value) => (identity = value),
      () => {
        // The identity line is decoration; the board works without it.
      },
    );
    const stop = startPoller({
      intervalMs: processIntervalMs,
      run: (signal) => sampleProcesses(processLimit, signal),
      onResult: applySample,
      onError: (reason) => {
        // Another scan was running: keep the last rows, the next tick retries.
        if (isApiError(reason, 'scan_busy')) return;
        processError = errorMessage(reason);
      },
    });
    const stopDetails = startPoller({
      intervalMs: detailIntervalMs,
      run: (signal) => fetchContainerDetails(signal),
      onResult: (values) =>
        (details = new Map(values.map((value) => [value.id, value]))),
      onError: () => {
        // Without detail the table still shows every live column.
      },
    });
    return () => {
      controller.abort();
      stop();
      stopDetails();
    };
  });
</script>

{#snippet readings(items: Reading[], label: string)}
  <dl class="readings" aria-label={label}>
    {#each items as item (item.label)}
      <div>
        <dt use:tooltip={item.hint ?? null}>{item.label}</dt>
        <dd class="num">{item.value}</dd>
      </div>
    {/each}
  </dl>
{/snippet}

{#snippet ifaceName(row: InterfaceRate)}<span class="mono strong"
    >{row.name}</span
  >{/snippet}
{#snippet ifaceRx(row: InterfaceRate)}<span class="num">{rate(row.rxBps)}</span
  >{/snippet}
{#snippet ifaceTx(row: InterfaceRate)}<span class="num">{rate(row.txBps)}</span
  >{/snippet}

{#snippet diskName(row: DiskDeviceRate)}<span class="mono strong"
    >{row.name}</span
  >{/snippet}
{#snippet diskRead(row: DiskDeviceRate)}<span class="num"
    >{rate(row.readBps)}</span
  >{/snippet}
{#snippet diskWrite(row: DiskDeviceRate)}<span class="num"
    >{rate(row.writeBps)}</span
  >{/snippet}
{#snippet diskOps(row: DiskDeviceRate)}<span class="num"
    >{iops(row.readIops)} · {iops(row.writeIops)}</span
  >{/snippet}

{#snippet fsMount(row: FilesystemObservation)}<span class="mono strong"
    >{row.mountPoint}</span
  >{/snippet}
{#snippet fsUsed(row: FilesystemObservation)}
  {@const used = row.usedPct ?? percent(row.usedBytes, row.totalBytes)}
  <span class={`num tone-${utilizationTone(used)}`}
    >{row.usedBytes == null ? missing : formatBytes(row.usedBytes)} · {pct(
      used,
    )}</span
  >
{/snippet}
{#snippet fsTotal(row: FilesystemObservation)}<span class="num"
    >{row.totalBytes == null ? missing : formatBytes(row.totalBytes)}</span
  >{/snippet}

{#snippet containerName(row: ContainerRow)}
  <a
    href={resourcePath(row.resourceId)}
    class="mono strong cell-clip"
    use:tooltip={row.image
      ? `${row.resourceName} · ${row.image}`
      : row.resourceName}>{row.name}</a
  >
{/snippet}
{#snippet containerStatus(row: ContainerRow)}
  <Status status={row.status} size="sm" />
{/snippet}
{#snippet containerUptime(row: ContainerRow)}
  <span class="num"
    >{row.uptimeSeconds == null
      ? missing
      : formatUptime(row.uptimeSeconds)}</span
  >
{/snippet}
{#snippet containerCpu(row: ContainerRow)}<span class="num"
    >{pct(row.cpuHostPct)}</span
  >{/snippet}
{#snippet containerMemory(row: ContainerRow)}<span class="num"
    >{memoryOfLimit(row)}</span
  >{/snippet}
{#snippet containerMemoryPct(row: ContainerRow)}<span
    class={`num tone-${utilizationTone(row.memoryPct)}`}
    >{pct(row.memoryPct)}</span
  >{/snippet}
{#snippet containerBlock(row: ContainerRow)}<span class="num"
    >{rwRates(row.blockReadBps, row.blockWriteBps)}</span
  >{/snippet}
{#snippet containerNet(row: ContainerRow)}<span class="num"
    >{rwRates(row.rxBps, row.txBps)}</span
  >{/snippet}
{#snippet containerPids(row: ContainerRow)}<span class="num"
    >{row.pids ?? missing}</span
  >{/snippet}
{#snippet containerPorts(row: ContainerRow)}
  {#if row.ports?.length}
    <span class="mono ports" use:tooltip={row.ports.join(', ')}
      >{row.ports.join(', ')}</span
    >
  {:else}<span class="muted">{missing}</span>{/if}
{/snippet}
{#snippet containerEmpty()}
  <EmptyState
    title="No containers"
    description="Containers appear here once the Docker collector lists them."
  />
{/snippet}

{#snippet processCpu(row: HostProcess)}<span
    class={`num tone-${utilizationTone(row.cpuPct, 50, 90)}`}
    >{pct(row.cpuPct)}</span
  >{/snippet}
{#snippet processMem(row: HostProcess)}<span class="num">{pct(row.memPct)}</span
  >{/snippet}
{#snippet processVirt(row: HostProcess)}<span class="num"
    >{row.virtBytes == null ? missing : formatBytes(row.virtBytes)}</span
  >{/snippet}
{#snippet processRes(row: HostProcess)}<span class="num"
    >{row.rssBytes == null ? missing : formatBytes(row.rssBytes)}</span
  >{/snippet}
{#snippet processPid(row: HostProcess)}<span class="num">{row.pid}</span
  >{/snippet}
{#snippet processUser(row: HostProcess)}<span class="mono"
    >{row.user || row.uid || missing}</span
  >{/snippet}
{#snippet processTime(row: HostProcess)}<span class="num"
    >{formatCpuTime(row.cpuTimeSeconds)}</span
  >{/snippet}
{#snippet processThreads(row: HostProcess)}<span class="num"
    >{row.threads ?? missing}</span
  >{/snippet}
{#snippet processNice(row: HostProcess)}<span class="num"
    >{row.nice ?? missing}</span
  >{/snippet}
{#snippet processState(row: HostProcess)}<span
    class="mono"
    use:tooltip={processStateLabel(row.state)}>{row.state || missing}</span
  >{/snippet}
{#snippet processCommand(row: HostProcess)}
  <span class="mono command" use:tooltip={row.command}
    >{#each commandParts(row.command) as part, index (index)}{#if part.redacted}<span
          class="redacted">{part.text}</span
        >{:else}{part.text}{/if}{/each}</span
  >
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
    title="Sampling processes"
    description="The first sample takes a moment; it refreshes every 5 seconds while this tab is open."
  >
    {#snippet icon()}<ListTree />{/snippet}
  </EmptyState>
{/snippet}

<div class="board">
  <p class="identity" data-testid="live-identity">
    <span class="hostname mono">{identity.hostname || 'This server'}</span>
    {#each identityLine as part (part)}
      <span class="sep" aria-hidden="true">·</span><span>{part}</span>
    {/each}
    <span class="sep" aria-hidden="true">·</span><span
      >up <span class="num">{formatUptime(host.uptimeSeconds)}</span></span
    >
  </p>

  <div class="grid three">
    <Card title="CPU">
      {#snippet actions()}<span class="num headline">{pct(host.cpuPct)}</span
        >{/snippet}
      {#if cores.length}
        <ol class="cores" aria-label="Busy share per core">
          {#each cores as value, index (index)}
            <li>
              <span class="core-label mono">cpu{index}</span>
              <span class="bar" aria-hidden="true"
                ><span
                  class={`fill tone-${utilizationTone(value)}`}
                  style:width={`${Math.min(100, Math.max(0, value))}%`}
                ></span></span
              >
              <span class="num core-value">{pct(value)}</span>
            </li>
          {/each}
        </ol>
      {/if}
      {@render readings(cpuReadings(host), 'CPU time')}
    </Card>

    <Card title="Memory">
      {#snippet actions()}<span class="num headline">{pct(host.memoryPct)}</span
        >{/snippet}
      {@render readings(memoryReadings(host), 'Memory')}
      <h3 class="sub">Swap <span class="num">{pct(host.swapPct)}</span></h3>
      {@render readings(swapReadings(host), 'Swap')}
    </Card>

    <Card title="Load and activity">
      {#snippet actions()}<span class="num headline"
          >{coreCount ? `${coreCount} cores` : missing}</span
        >{/snippet}
      {@render readings(loadReadings(host, coreCount), 'Load average')}
      <h3 class="sub">Activity</h3>
      {@render readings(activityReadings(host), 'System activity')}
      {#if tasks}
        <h3 class="sub">Tasks</h3>
        {@render readings(
          [
            { label: 'total', value: String(tasks.total) },
            { label: 'threads', value: String(tasks.threads) },
            { label: 'running', value: String(tasks.running) },
            { label: 'sleeping', value: String(tasks.sleeping) },
            { label: 'other', value: String(tasks.other) },
          ],
          'Tasks',
        )}
      {/if}
    </Card>
  </div>

  <div class="grid three">
    <Card title="Network" padded={false}>
      <DataTable
        rows={interfaces}
        columns={interfaceColumns}
        rowKey={(row) => row.name}
        caption="Network interfaces"
        density="compact"
      />
    </Card>
    <Card title="Disk I/O" padded={false}>
      <DataTable
        rows={disks}
        columns={diskColumns}
        rowKey={(row) => row.name}
        caption="Disk I/O per device"
        density="compact"
      />
    </Card>
    <Card title="Filesystems" padded={false}>
      <DataTable
        rows={filesystems}
        columns={fsColumns}
        rowKey={(row) => row.mountKey}
        caption="Filesystems"
        density="compact"
      />
    </Card>
  </div>

  <Card title="Containers" padded={false}>
    {#snippet actions()}<span class="meta num"
        >{running} running · {containers.length} total</span
      >{/snippet}
    <DataTable
      rows={containers}
      columns={containerColumns}
      rowKey={(row) => row.id}
      caption="Containers"
      bind:sort={containerSort}
      empty={containerEmpty}
      density="compact"
    />
  </Card>

  <Card title="Processes" padded={false}>
    {#snippet actions()}
      {#if sampledAt}<span class="meta"
          >Top {processLimit} ·
          <span class="num">{formatClock(sampledAt)}</span>
          ·
          <RelativeTime value={sampledAt} /></span
        >{/if}
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
      empty={processEmpty}
      density="compact"
    />
    {#snippet footer()}
      <p class="note">
        Refreshes every 5 seconds while this tab is open and visible; nothing is
        stored. Command lines are redacted on the server.
      </p>
    {/snippet}
  </Card>
</div>

<style>
  .board {
    display: grid;
    gap: var(--space-4);
    min-width: 0;
  }
  .grid {
    display: grid;
    gap: var(--space-4);
    min-width: 0;
  }
  .grid.three {
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 300px), 1fr));
  }
  .identity {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-1) var(--space-2);
    margin: 0;
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .hostname {
    color: var(--fg);
    font-weight: 600;
  }
  .sep {
    color: var(--line-strong);
  }
  .headline {
    color: var(--fg);
    font-weight: 600;
  }
  .meta {
    color: var(--muted);
    font-size: var(--text-mono);
  }
  .readings {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-1) var(--space-4);
    margin: 0;
    font-size: var(--text-mono);
  }
  .readings div {
    display: flex;
    justify-content: space-between;
    gap: var(--space-2);
    min-width: 0;
  }
  .readings dt {
    color: var(--muted);
  }
  .readings dd {
    margin: 0;
    white-space: nowrap;
  }
  .sub {
    display: flex;
    justify-content: space-between;
    margin: var(--space-4) 0 var(--space-2);
    color: var(--muted);
    font-size: var(--text-mono);
    font-weight: 500;
  }
  .cores {
    display: grid;
    gap: 3px;
    margin: 0 0 var(--space-3);
    padding: 0;
    list-style: none;
    font-size: var(--text-mono);
  }
  .cores li {
    display: grid;
    grid-template-columns: 4.5ch 1fr 6ch;
    align-items: center;
    gap: var(--space-2);
  }
  .core-label {
    color: var(--muted);
  }
  .core-value {
    text-align: right;
  }
  /* Plain width, no transition: a bar per core redraws every 2 s and must
     not animate (see the live-motion notes in Gauge.svelte). */
  .bar {
    position: relative;
    height: 6px;
    overflow: hidden;
    border-radius: 1px;
    background: var(--line);
  }
  .fill {
    position: absolute;
    inset: 0 auto 0 0;
    background: var(--accent);
  }
  .fill.tone-warn {
    background: var(--warn-solid);
  }
  .fill.tone-critical {
    background: var(--critical-solid);
  }
  .tone-warn:not(.fill) {
    color: var(--warn-fg);
  }
  .tone-critical:not(.fill) {
    color: var(--critical-fg);
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
  .cell-clip {
    display: inline-block;
    max-width: min(220px, 100%);
    overflow: hidden;
    text-overflow: ellipsis;
    vertical-align: middle;
    white-space: nowrap;
  }
  .ports {
    display: inline-block;
    max-width: min(140px, 100%);
    overflow: hidden;
    text-overflow: ellipsis;
    vertical-align: middle;
    white-space: nowrap;
  }
  .command {
    display: inline-block;
    max-width: min(480px, 100%);
    overflow: hidden;
    text-overflow: ellipsis;
    vertical-align: middle;
    white-space: nowrap;
  }
  .redacted {
    color: var(--muted);
    font-style: italic;
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
</style>
