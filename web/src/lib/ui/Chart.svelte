<script lang="ts" module>
  export interface ChartPoint {
    /** Unix seconds. */
    at: number;
    value: number | null;
  }
  export interface ChartSeries {
    key: string;
    label: string;
    points: ChartPoint[];
    /** 1-based index into the categorical chart palette. */
    color?: number;
  }
  export interface ChartGap {
    from: string;
    to: string;
    reason: string;
  }
  export interface ChartMarker {
    /** Unix seconds. */
    at: number;
    label: string;
    count?: number;
    href?: string;
  }
</script>

<script lang="ts">
  import { onMount, tick } from 'svelte';
  import uPlot from 'uplot';
  import 'uplot/dist/uPlot.min.css';
  import {
    invalidateChartColors,
    readChartColors,
    seriesColor as paletteColor,
  } from './chart-colors';
  import { uniqueId } from './focus';
  import { formatAbsolute } from './relative-time';

  let {
    series,
    gaps = [],
    markers = [],
    formatValue = (value: number) => String(Math.round(value * 10) / 10),
    height = 220,
    variant = 'line',
    min = 0,
    max,
    label,
    legend = true,
  }: {
    series: ChartSeries[];
    gaps?: ChartGap[];
    markers?: ChartMarker[];
    formatValue?: (value: number) => string;
    height?: number;
    variant?: 'line' | 'area';
    /** Fixed y-axis minimum; pass null to auto-scale. */
    min?: number | null;
    max?: number;
    /** Accessible name for the chart. */
    label: string;
    /** Series names with the hovered or latest value, in mono. */
    legend?: boolean;
  } = $props();

  let root = $state<HTMLDivElement | null>(null);
  let wrapper = $state<HTMLDivElement | null>(null);
  let plot: uPlot | undefined;
  let hover = $state<{
    index: number;
    left: number;
    top: number;
    notes: string[];
  } | null>(null);
  // Resolved once per theme (see chart-colors.ts), not on every hover.
  let colors = $state.raw(readChartColors());
  let selected = $state<number | null>(null);
  let width = $state(0);
  const inspectorId = uniqueId('chart-inspector');

  const aligned = $derived.by<uPlot.AlignedData>(() => {
    if (!series.length) return [[], []];
    const tables = series.map(
      (item) =>
        [
          item.points.map((point) => point.at),
          item.points.map((point) => point.value),
        ] as uPlot.AlignedData,
    );
    return tables.length === 1 ? tables[0] : uPlot.join(tables);
  });

  const xs = $derived(aligned[0] as number[]);

  let measureContext: CanvasRenderingContext2D | null = null;
  /** Widest formatted tick label in CSS pixels, plus room for the gap. */
  function axisWidth(values: string[] | null | undefined): number {
    if (!values?.length) return 60;
    measureContext ??= document.createElement('canvas').getContext('2d');
    if (!measureContext) return 60;
    measureContext.font = '11px JetBrains Mono, monospace';
    const widest = Math.max(
      ...values.map((value) => measureContext!.measureText(value).width),
    );
    return Math.ceil(widest) + 20;
  }

  function seriesColor(index: number, item: ChartSeries): string {
    return paletteColor(colors, index, item.color);
  }

  function withAlpha(color: string, alpha: number): string {
    return `color-mix(in srgb, ${color} ${Math.round(alpha * 100)}%, transparent)`;
  }

  function latest(points: Array<number | null>): number | null {
    for (let index = points.length - 1; index >= 0; index -= 1) {
      const value = points[index];
      if (value != null) return value;
    }
    return null;
  }

  /** Annotations within a few pixels of the cursor, as tooltip notes. */
  function markerNotes(u: uPlot, left: number): string[] {
    return markers
      .filter((marker) => Math.abs(u.valToPos(marker.at, 'x') - left) <= 6)
      .map((marker) =>
        marker.count && marker.count > 1
          ? `${marker.label} (${marker.count})`
          : marker.label,
      );
  }

  function options(): uPlot.Options {
    const {
      axis: axisColor,
      grid,
      gap: gapFill,
      marker: markerFill,
      area,
    } = colors;
    return {
      width: Math.max(1, width),
      height,
      legend: { show: false },
      cursor: {
        y: false,
        points: { size: 7, width: 2 },
        drag: { x: false, y: false },
      },
      scales: {
        y: {
          range: (_u, dataMin, dataMax) => {
            const lo = min == null ? dataMin : Math.min(min, dataMin ?? min);
            const hi =
              max ??
              (dataMax == null
                ? lo + 1
                : dataMax === lo
                  ? lo + 1
                  : dataMax * 1.08);
            return [lo, hi];
          },
        },
      },
      series: [
        {},
        ...series.map((item, index) => {
          const color = seriesColor(index, item);
          return {
            label: item.label,
            stroke: color,
            width: 1.5,
            spanGaps: false,
            points: { show: false, fill: color, stroke: color },
            fill:
              variant === 'area' && series.length === 1
                ? withAlpha(color, area)
                : undefined,
          } satisfies uPlot.Series;
        }),
      ],
      axes: [
        {
          stroke: axisColor,
          grid: { stroke: grid, width: 1 },
          ticks: { stroke: grid, width: 1 },
          font: '11px JetBrains Mono, monospace',
          // Room above the tick labels for the annotation triangles.
          gap: markers.length ? 12 : 6,
          size: markers.length ? 34 : 28,
        },
        {
          stroke: axisColor,
          grid: { stroke: grid, width: 1 },
          ticks: { show: false },
          font: '11px JetBrains Mono, monospace',
          gap: 8,
          size: (_u, values) => axisWidth(values),
          values: (_u, values) => values.map((value) => formatValue(value)),
        },
      ],
      hooks: {
        draw: [
          (u) => {
            const ctx = u.ctx;
            ctx.save();
            ctx.fillStyle = gapFill;
            for (const gap of gaps) {
              const left = Math.round(
                u.valToPos(new Date(gap.from).getTime() / 1000, 'x', true),
              );
              const right = Math.round(
                u.valToPos(new Date(gap.to).getTime() / 1000, 'x', true),
              );
              ctx.fillRect(
                left,
                u.bbox.top,
                Math.max(2, right - left),
                u.bbox.height,
              );
            }
            // Annotations: small brass triangles just under the x axis line.
            ctx.fillStyle = markerFill;
            const ratio = window.devicePixelRatio || 1;
            const half = 4 * ratio;
            const base = u.bbox.top + u.bbox.height + 2 * ratio;
            for (const marker of markers) {
              const x = Math.round(u.valToPos(marker.at, 'x', true));
              ctx.beginPath();
              ctx.moveTo(x, base);
              ctx.lineTo(x + half, base + half * 1.5);
              ctx.lineTo(x - half, base + half * 1.5);
              ctx.closePath();
              ctx.fill();
            }
            ctx.restore();
          },
        ],
        setCursor: [
          (u) => {
            const index = u.cursor.idx;
            if (index == null || index < 0) {
              hover = null;
              return;
            }
            const left = u.cursor.left ?? 0;
            hover = {
              index,
              left,
              top: u.cursor.top ?? 0,
              notes: markerNotes(u, left),
            };
          },
        ],
      },
    };
  }

  function rebuild() {
    if (!root) return;
    plot?.destroy();
    plot = new uPlot(options(), aligned, root);
    // uPlot paints the cursor with its own CSS variables.
    root.style.setProperty('--uplot-cursor', colors.cursor);
  }

  onMount(() => {
    if (!root || !wrapper) return;
    width = wrapper.clientWidth;
    rebuild();
    const resize = new ResizeObserver(() => {
      if (!wrapper) return;
      width = wrapper.clientWidth;
      plot?.setSize({ width: Math.max(1, width), height });
    });
    resize.observe(wrapper);
    const theme = new MutationObserver(() => {
      invalidateChartColors();
      colors = readChartColors();
      rebuild();
    });
    theme.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme'],
    });
    return () => {
      resize.disconnect();
      theme.disconnect();
      plot?.destroy();
    };
  });

  let firstRender = true;
  $effect(() => {
    void aligned;
    void gaps;
    void markers;
    if (firstRender) {
      firstRender = false;
      return;
    }
    if (plot) {
      plot.setData(aligned);
      plot.redraw();
    }
  });

  const hoverValues = $derived.by(() => {
    if (!hover) return null;
    const at = xs[hover.index];
    return {
      at,
      notes: hover.notes,
      values: series.map((item, index) => ({
        label: item.label,
        color: seriesColor(index, item),
        value:
          (aligned[index + 1] as Array<number | null>)[hover!.index] ?? null,
      })),
    };
  });

  const tooltipStyle = $derived.by(() => {
    if (!hover) return '';
    const flip = hover.left > width * 0.6;
    return `left:${hover.left}px;top:${Math.max(8, hover.top - 8)}px;transform:translate(${flip ? 'calc(-100% - 12px)' : '12px'}, -100%)`;
  });

  function stats(item: ChartSeries) {
    const values = item.points.flatMap((point) =>
      point.value == null ? [] : [point.value],
    );
    if (!values.length) return null;
    return {
      min: Math.min(...values),
      max: Math.max(...values),
      avg: values.reduce((sum, value) => sum + value, 0) / values.length,
    };
  }

  function inspect(event: KeyboardEvent) {
    if (!xs.length) return;
    let next = selected ?? xs.length - 1;
    if (event.key === 'ArrowLeft') next = Math.max(0, next - 1);
    else if (event.key === 'ArrowRight')
      next = Math.min(xs.length - 1, next + 1);
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = xs.length - 1;
    else return;
    event.preventDefault();
    selected = next;
    void tick().then(() => {
      if (!plot) return;
      const left = plot.valToPos(xs[next], 'x');
      plot.setCursor({ left, top: plot.bbox.height / 2 / devicePixelRatio });
    });
  }
</script>

<div class="chart" bind:this={wrapper} style:height={`${height}px`}>
  <div class="canvas" bind:this={root} aria-hidden="true"></div>
  {#if hoverValues}
    <div class="tooltip" style={tooltipStyle} aria-hidden="true">
      <div class="tooltip-time">{formatAbsolute(hoverValues.at * 1000)}</div>
      {#each hoverValues.values as entry (entry.label)}
        <div class="tooltip-row">
          <span class="swatch" style:background={entry.color}></span>
          <span class="tooltip-label">{entry.label}</span>
          <span class="tooltip-value"
            >{entry.value == null ? 'no data' : formatValue(entry.value)}</span
          >
        </div>
      {/each}
      {#each hoverValues.notes as note (note)}
        <div class="tooltip-note">▲ {note}</div>
      {/each}
    </div>
  {/if}
</div>

{#if legend && series.length}
  <ul class="legend" aria-hidden="true">
    {#each series as item, index (item.key)}
      {@const points = aligned[index + 1] as Array<number | null>}
      {@const current = hover ? (points[hover.index] ?? null) : latest(points)}
      <li>
        <span class="swatch" style:background={seriesColor(index, item)}></span>
        <span class="legend-label">{item.label}</span>
        <span class="legend-value"
          >{current == null ? '—' : formatValue(current)}</span
        >
      </li>
    {/each}
  </ul>
{/if}

<button
  type="button"
  class="inspector"
  id={inspectorId}
  aria-label={`Inspect ${label} with the arrow keys`}
  onkeydown={inspect}
  onfocus={() => (selected ??= xs.length - 1)}
>
  <span class="inspector-hint">Inspect</span>
  <span class="sr-only">
    {#each series as item (item.key)}
      {@const summary = stats(item)}
      {item.label}: {summary
        ? `minimum ${formatValue(summary.min)}, average ${formatValue(summary.avg)}, maximum ${formatValue(summary.max)}.`
        : 'no measurements.'}
    {/each}
    {#if gaps.length}{gaps.length} data {gaps.length === 1
        ? 'gap'
        : 'gaps'}.{/if}
  </span>
  {#if selected != null && xs[selected] != null}
    <span class="inspector-readout" role="status">
      {formatAbsolute(xs[selected] * 1000)}:
      {#each series as item, index (item.key)}
        {@const value = (aligned[index + 1] as Array<number | null>)[selected]}
        {item.label}
        {value == null ? 'no data' : formatValue(value)}{index <
        series.length - 1
          ? ', '
          : ''}
      {/each}
    </span>
  {/if}
</button>
{#if markers.length}
  <ul class="sr-only" aria-label="Chart event annotations">
    {#each markers as marker (marker.at + marker.label)}
      <li>
        {#if marker.href}<a href={marker.href}
            >{formatAbsolute(marker.at * 1000)}: {marker.label}</a
          >
        {:else}{formatAbsolute(marker.at * 1000)}: {marker.label}{/if}
      </li>
    {/each}
  </ul>
{/if}

<style>
  .chart {
    position: relative;
    width: 100%;
  }
  .canvas :global(.uplot) {
    font-family: var(--font-mono);
  }
  .canvas :global(.u-cursor-x) {
    border-right: 1px solid var(--chart-cursor);
  }
  .tooltip {
    position: absolute;
    z-index: 5;
    min-width: 150px;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-control);
    background: var(--panel);
    box-shadow: var(--shadow-overlay);
    font-size: var(--text-meta);
    pointer-events: none;
    white-space: nowrap;
  }
  .tooltip-time {
    margin-bottom: 4px;
    color: var(--muted);
    font-family: var(--font-mono);
  }
  .tooltip-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .swatch {
    width: 8px;
    height: 8px;
    border-radius: 2px;
  }
  .tooltip-label {
    color: var(--muted);
  }
  .tooltip-value {
    margin-left: auto;
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
  }
  .tooltip-note {
    margin-top: 4px;
    color: var(--accent-text);
  }
  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-4);
    margin: var(--space-2) 0 0;
    padding: 0;
    list-style: none;
    font-size: var(--text-meta);
  }
  .legend li {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }
  .legend-label {
    color: var(--muted);
  }
  .legend-value {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    font-variant-numeric: tabular-nums;
  }
  .inspector {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-2);
    padding: 2px 8px;
    border: 1px dashed var(--line-strong);
    border-radius: var(--radius-control);
    background: none;
    color: var(--muted);
    font-size: var(--text-meta);
    opacity: 0;
  }
  .inspector:focus-visible {
    opacity: 1;
    color: var(--fg);
    outline-offset: 2px;
  }
  .inspector-readout {
    font-family: var(--font-mono);
  }
</style>
