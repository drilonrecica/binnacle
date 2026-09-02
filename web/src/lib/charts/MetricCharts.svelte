<script lang="ts" module>
  import type { Metric } from '../history';
  export type Formatter = 'percent' | 'bytes' | 'rate' | 'load' | 'iops';
  export interface ChartGroup {
    key: string;
    title: string;
    description?: string;
    metrics: Array<{ metric: Metric; label: string; color?: number }>;
    format: Formatter;
    /** Fixed y maximum, e.g. 100 for percentages. */
    max?: number;
    variant?: 'line' | 'area';
  }
</script>

<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchMetrics } from '../api/metrics';
  import { listEvents } from '../api/events';
  import { errorMessage } from '../api/client';
  import { boundAnnotations, type ChartAnnotation } from '../annotations';
  import { chartPoints, type HistoryResponse } from '../history';
  import { formatBytes, formatNumber, formatRate } from '../i18n';
  import { prefs } from '../preferences.svelte';
  import { resourcePath } from '../router';
  import Card from '../ui/Card.svelte';
  import Chart, { type ChartSeries } from '../ui/Chart.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import TimeRangePicker, {
    type TimeRange,
  } from '../ui/TimeRangePicker.svelte';
  import { tooltip } from '../ui/tooltip';

  let {
    scope,
    id,
    groups,
    columns = 2,
  }: {
    scope: 'host' | 'resource';
    id?: string;
    groups: ChartGroup[];
    columns?: 1 | 2;
  } = $props();

  let range = $state<TimeRange | undefined>(undefined);
  let data = $state<Record<string, HistoryResponse>>({});
  let errors = $state<Record<string, string>>({});
  let loading = $state(true);
  let markers = $state<ChartAnnotation[]>([]);
  let controller: AbortController | undefined;
  let picker = $state<TimeRangePicker | null>(null);

  const formatters: Record<Formatter, (value: number) => string> = {
    percent: (value) => `${formatNumber(value)}%`,
    bytes: (value) => formatBytes(value),
    rate: (value) => formatRate(value),
    load: (value) => formatNumber(value),
    iops: (value) => `${formatNumber(value)} IOPS`,
  };

  async function load(current: TimeRange) {
    controller?.abort();
    controller = new AbortController();
    const signal = controller.signal;
    loading = true;
    errors = {};
    const results = await Promise.all(
      groups.map(async (group) => {
        try {
          const response = await fetchMetrics(
            scope,
            group.metrics.map((item) => item.metric),
            current.from,
            current.to,
            id,
            signal,
          );
          return [group.key, response, ''] as const;
        } catch (reason) {
          if (signal.aborted) return [group.key, null, ''] as const;
          return [group.key, null, errorMessage(reason)] as const;
        }
      }),
    );
    if (signal.aborted) return;
    const nextData: Record<string, HistoryResponse> = {};
    const nextErrors: Record<string, string> = {};
    for (const [key, response, error] of results) {
      if (response) nextData[key] = response;
      if (error) nextErrors[key] = error;
    }
    data = nextData;
    errors = nextErrors;
    loading = false;
    try {
      const events = await listEvents(
        {
          from: current.from,
          to: current.to,
          resourceId: scope === 'resource' ? id : undefined,
        },
        signal,
      );
      markers = boundAnnotations(
        events.map((event) => ({
          id: event.id,
          ts: event.ts,
          type: event.type,
          summary: event.summary,
          resourceId: event.resourceId,
        })),
        current.from,
        current.to,
      ).map((marker) => ({
        ...marker,
        href: marker.href.replace('/events', '/activity'),
      }));
    } catch {
      markers = [];
    }
  }

  function seriesFor(group: ChartGroup): ChartSeries[] {
    const response = data[group.key];
    if (!response) return [];
    return group.metrics.map((item, index) => {
      const series = response.series.find(
        (entry) => entry.metric === item.metric,
      );
      return {
        key: item.metric,
        label: item.label,
        color: item.color ?? index + 1,
        points: series ? chartPoints(series, response.gaps) : [],
      };
    });
  }

  function latest(group: ChartGroup, metric: Metric): number | null {
    const series = data[group.key]?.series.find(
      (entry) => entry.metric === metric,
    );
    for (let index = (series?.points.length ?? 0) - 1; index >= 0; index--) {
      const value = series?.points[index]?.avg;
      if (value != null) return value;
    }
    return null;
  }

  function summary(group: ChartGroup) {
    const series = data[group.key]?.series[0];
    if (!series) return null;
    const measured = series.points.filter(
      (point) => point.avg != null && point.count > 0,
    );
    if (!measured.length) return null;
    const count = measured.reduce((sum, point) => sum + point.count, 0);
    return {
      min: Math.min(...measured.map((point) => point.min ?? point.avg ?? 0)),
      max: Math.max(...measured.map((point) => point.max ?? point.avg ?? 0)),
      avg:
        measured.reduce(
          (sum, point) => sum + (point.avg ?? 0) * point.count,
          0,
        ) / count,
    };
  }

  const resolution = $derived(Object.values(data)[0]?.resolution ?? '');
  const gapCount = $derived(Object.values(data)[0]?.gaps.length ?? 0);

  onMount(() => {
    const timer = window.setInterval(() => picker?.refresh(), 60_000);
    return () => {
      window.clearInterval(timer);
      controller?.abort();
    };
  });

  void resourcePath;
</script>

<div class="toolbar">
  <TimeRangePicker
    bind:this={picker}
    bind:value={range}
    defaultKey={prefs.value.chartRange}
    onchange={(next) => void load(next)}
  />
  <div class="meta">
    {#if resolution}
      <span use:tooltip={'Each point averages samples at this resolution'}
        >Resolution {resolution}</span
      >
    {/if}
    {#if gapCount}
      <span
        class="gaps"
        use:tooltip={'Shaded areas mark missing data; hover a chart to see why'}
        >{gapCount} {gapCount === 1 ? 'gap' : 'gaps'}</span
      >
    {/if}
    {#if markers.length}
      <span>{markers.length} {markers.length === 1 ? 'event' : 'events'}</span>
    {/if}
  </div>
</div>

<div class={`grid cols-${columns}`}>
  {#each groups as group (group.key)}
    {@const stats = summary(group)}
    <Card title={group.title} description={group.description} padded={false}>
      <div class="body">
        <dl class="latest">
          {#each group.metrics as item, index (item.metric)}
            {@const value = latest(group, item.metric)}
            <div>
              <dt>
                <span
                  class="swatch"
                  style:background={`var(--chart-${item.color ?? index + 1})`}
                  aria-hidden="true"
                ></span>{item.label}
              </dt>
              <dd class="num">
                {value == null ? '—' : formatters[group.format](value)}
              </dd>
            </div>
          {/each}
        </dl>
        {#if loading && !data[group.key]}
          <Skeleton height={220} />
        {:else if errors[group.key]}
          <p class="error" role="alert">{errors[group.key]}</p>
        {:else}
          <Chart
            label={group.title}
            series={seriesFor(group)}
            gaps={data[group.key]?.gaps ?? []}
            {markers}
            formatValue={formatters[group.format]}
            max={group.max}
            variant={group.variant ??
              (group.metrics.length === 1 ? 'area' : 'line')}
          />
          {#if stats}
            <p class="stats num">
              <span>min {formatters[group.format](stats.min)}</span>
              <span>avg {formatters[group.format](stats.avg)}</span>
              <span>max {formatters[group.format](stats.max)}</span>
            </p>
          {/if}
        {/if}
      </div>
    </Card>
  {/each}
</div>

<style>
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }
  .meta {
    display: flex;
    gap: var(--space-4);
    color: var(--text-3);
    font-size: var(--text-xs);
    font-family: var(--font-mono);
  }
  .gaps {
    color: var(--warn-fg);
  }
  .grid {
    display: grid;
    gap: var(--space-4);
  }
  .cols-2 {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .latest {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-4);
    margin: 0 0 var(--space-2);
    padding: 0 var(--space-1);
    font-size: var(--text-xs);
  }
  .latest div {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .latest dt {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--text-2);
  }
  .latest dd {
    margin: 0;
    color: var(--text);
    font-weight: 500;
  }
  .swatch {
    width: 8px;
    height: 8px;
    border-radius: 2px;
  }
  .body {
    padding: var(--space-3) var(--space-3) var(--space-2);
  }
  .error {
    padding: var(--space-6) var(--space-4);
    color: var(--critical-fg);
    font-size: var(--text-sm);
    text-align: center;
  }
  .stats {
    display: flex;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-2) 0;
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  @media (max-width: 1100px) {
    .cols-2 {
      grid-template-columns: 1fr;
    }
  }
</style>
