<script lang="ts">
  import Cpu from '@lucide/svelte/icons/cpu';
  import MemoryStick from '@lucide/svelte/icons/memory-stick';
  import HardDrive from '@lucide/svelte/icons/hard-drive';
  import ArrowDownUp from '@lucide/svelte/icons/arrow-down-up';
  import type { LiveSnapshot } from '../live.svelte';
  import { liveSeries, percent } from '../live-series.svelte';
  import { bucketize } from '../series';
  import { formatBytes, formatNumber, formatRate } from '../i18n';
  import { formatUptime } from '../watch';
  import Stat from '../ui/Stat.svelte';
  import { utilizationTone } from '../ui/status';

  let {
    snapshot,
    windowMs = 3_600_000,
  }: { snapshot: LiveSnapshot; windowMs?: number } = $props();

  const host = $derived(snapshot.host);
  const now = $derived(new Date(snapshot.ts).getTime());
  const spark = (key: 'cpu' | 'memory' | 'rx' | 'tx') =>
    bucketize(
      liveSeries.hostWindow(key, windowMs, now),
      now - windowMs,
      now,
      60,
    );

  const memoryPct = $derived(
    host.memoryPct ?? percent(host.memoryUsedBytes, host.memoryTotalBytes),
  );
  const memoryFree = $derived(
    host.memoryAvailableBytes ??
      (host.memoryTotalBytes != null && host.memoryUsedBytes != null
        ? host.memoryTotalBytes - host.memoryUsedBytes
        : null),
  );

  const root = $derived(
    snapshot.filesystems?.find((mount) => mount.mountPoint === '/') ??
      (host.diskTotalBytes != null
        ? {
            mountPoint: '/',
            usedBytes: host.diskUsedBytes,
            totalBytes: host.diskTotalBytes,
            usedPct: percent(host.diskUsedBytes, host.diskTotalBytes),
            availableBytes: null,
          }
        : null),
  );
  const worstOther = $derived(
    [...(snapshot.filesystems ?? [])]
      .filter((mount) => mount.mountPoint !== '/' && mount.usedPct != null)
      .sort((a, b) => (b.usedPct ?? 0) - (a.usedPct ?? 0))[0] ?? null,
  );
  const diskPct = $derived(root?.usedPct ?? null);
  const diskFree = $derived(
    root?.availableBytes ??
      (root?.totalBytes != null && root.usedBytes != null
        ? root.totalBytes - root.usedBytes
        : null),
  );

  function pct(value: number | null | undefined) {
    return value == null || !Number.isFinite(value) ? '—' : formatNumber(value);
  }
</script>

<section class="headroom" aria-labelledby="headroom-title">
  <h2 id="headroom-title" class="sr-only">Host headroom</h2>
  <div class="tiles">
    <Stat
      label="CPU"
      value={pct(host.cpuPct)}
      unit={host.cpuPct == null ? '' : '%'}
      percent={host.cpuPct ?? null}
      secondary={host.load1 == null
        ? 'Load unavailable'
        : `load ${formatNumber(host.load1)} · ${formatNumber(host.load5)} · ${formatNumber(host.load15)}`}
      sparkline={spark('cpu')}
      sparklineMax={100}
      sparklineLabel="CPU over the last hour"
      href="/host"
    >
      {#snippet icon()}<Cpu />{/snippet}
    </Stat>
    <Stat
      label="Memory"
      value={host.memoryUsedBytes == null
        ? '—'
        : formatBytes(host.memoryUsedBytes)}
      percent={memoryPct}
      secondary={host.memoryTotalBytes == null
        ? 'Total unavailable'
        : `${pct(memoryPct)}% of ${formatBytes(host.memoryTotalBytes)} · ${memoryFree == null ? '—' : formatBytes(memoryFree)} free`}
      sparkline={spark('memory')}
      sparklineMax={100}
      sparklineLabel="Memory over the last hour"
      href="/host"
    >
      {#snippet icon()}<MemoryStick />{/snippet}
    </Stat>
    <Stat
      label={root ? `Disk ${root.mountPoint}` : 'Disk'}
      value={root?.usedBytes == null ? '—' : formatBytes(root.usedBytes)}
      percent={diskPct}
      secondary={root?.totalBytes == null
        ? 'Filesystem data unavailable'
        : `${pct(diskPct)}% of ${formatBytes(root.totalBytes)} · ${diskFree == null ? '—' : formatBytes(diskFree)} free`}
      href="/host?tab=filesystems"
    >
      {#snippet icon()}<HardDrive />{/snippet}
      {#if worstOther}
        <p class={`mount ${utilizationTone(worstOther.usedPct)}`}>
          <span class="mount-dot" aria-hidden="true"></span>
          <span class="mount-path">{worstOther.mountPoint}</span>
          <span class="mount-pct num">{pct(worstOther.usedPct)}%</span>
        </p>
      {/if}
    </Stat>
    <Stat
      label="Network"
      value={host.networkRxBps == null ? '—' : formatRate(host.networkRxBps)}
      secondary={host.networkTxBps == null
        ? 'Transmit unavailable'
        : `↓ receive · ↑ ${formatRate(host.networkTxBps)}`}
      sparkline={spark('rx')}
      sparklineLabel="Receive rate over the last hour"
      tone="accent"
      href="/host"
    >
      {#snippet icon()}<ArrowDownUp />{/snippet}
    </Stat>
  </div>
  <dl class="meta">
    <div>
      <dt>Uptime</dt>
      <dd class="num">{formatUptime(host.uptimeSeconds)}</dd>
    </div>
    <div>
      <dt>Boot</dt>
      <dd class="num" title={snapshot.bootIdentity}>
        {snapshot.bootIdentity || '—'}
      </dd>
    </div>
    <div>
      <dt>Collectors</dt>
      <dd class="collectors">
        {#each Object.entries(snapshot.collectors) as [name, collector] (name)}
          <span
            class={`collector ${collector.state}`}
            title={collector.reason ?? collector.state}
          >
            <span class="collector-dot" aria-hidden="true"></span>{name}
            <span class="sr-only">{collector.state}</span>
          </span>
        {/each}
      </dd>
    </div>
  </dl>
</section>

<style>
  .headroom {
    margin-bottom: var(--space-5);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    box-shadow: var(--shadow-sm);
    overflow: hidden;
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
  .tiles > :global(.stat) {
    border-right: 1px solid var(--border);
  }
  .tiles > :global(.stat:last-child) {
    border-right: 0;
  }
  .mount {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: 2px;
    font-size: var(--text-xs);
    color: var(--text-2);
  }
  .mount-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--mount-color, var(--neutral-solid));
  }
  .mount.warn {
    --mount-color: var(--warn-solid);
  }
  .mount.critical {
    --mount-color: var(--critical-solid);
    color: var(--critical-fg);
  }
  .mount.ok {
    --mount-color: var(--ok-solid);
  }
  .mount-path {
    font-family: var(--font-mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .mount-pct {
    margin-left: auto;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-6);
    margin: 0;
    padding: var(--space-2) var(--space-4);
    border-top: 1px solid var(--border);
    background: var(--bg-subtle);
    font-size: var(--text-xs);
  }
  .meta div {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  dt {
    color: var(--text-3);
  }
  dd {
    margin: 0;
    color: var(--text-2);
  }
  .collectors {
    display: inline-flex;
    gap: var(--space-3);
  }
  .collector {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }
  .collector-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--neutral-solid);
  }
  .collector.healthy .collector-dot {
    background: var(--ok-solid);
  }
  .collector.degraded .collector-dot {
    background: var(--warn-solid);
  }
  .collector.down .collector-dot {
    background: var(--critical-solid);
  }
  @media (max-width: 1100px) {
    .tiles {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .tiles > :global(.stat:nth-child(2)) {
      border-right: 0;
    }
    .tiles > :global(.stat:nth-child(-n + 2)) {
      border-bottom: 1px solid var(--border);
    }
  }
  @media (max-width: 560px) {
    .tiles {
      grid-template-columns: 1fr;
    }
    .tiles > :global(.stat) {
      border-right: 0;
      border-bottom: 1px solid var(--border);
    }
    .tiles > :global(.stat:last-child) {
      border-bottom: 0;
    }
  }
</style>
