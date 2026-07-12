<script lang="ts">
  import { onMount } from 'svelte';
  import uPlot from 'uplot';
  import 'uplot/dist/uPlot.min.css';
  import { summary, toSeries, type Point } from '../chart';
  type Gap = { from: string; to: string; reason: string };
  type Marker = { at: number; label: string; count?: number; href?: string };
  let {
    points,
    label,
    variant = 'line',
    gaps = [],
    markers = [],
  }: {
    points: Point[];
    label: string;
    variant?: 'line' | 'area' | 'sparkline';
    gaps?: Gap[];
    markers?: Marker[];
  } = $props();
  let root: HTMLDivElement;
  let plot: uPlot | undefined;
  let selected = $state(0);
  let cursorText = $state('');
  let tooltipLeft = $state(0);
  let tooltipTop = $state(0);
  function color(name: string, fallback: string): string {
    return (
      getComputedStyle(document.documentElement)
        .getPropertyValue(name)
        .trim() || fallback
    );
  }
  function options(): uPlot.Options {
    const line = color('--chart-1', '#2ed0d0');
    const text = color('--muted', '#82949e');
    const grid = color('--border', '#243744');
    return {
      width: root.clientWidth || 1,
      height: variant === 'sparkline' ? 48 : 180,
      legend: { show: false },
      series: [
        {},
        {
          label,
          stroke: line,
          points: { fill: line, stroke: line },
          fill: variant === 'area' ? 'rgb(120 220 232 / .2)' : undefined,
        },
      ],
      axes:
        variant === 'sparkline'
          ? []
          : [
              { stroke: text, grid: { stroke: grid }, ticks: { stroke: grid } },
              { stroke: text, grid: { stroke: grid }, ticks: { stroke: grid } },
            ],
      plugins: [
        {
          hooks: {
            draw: [
              (u) => {
                const ctx = u.ctx;
                ctx.save();
                ctx.fillStyle = 'rgba(245,196,81,.10)';
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
                ctx.strokeStyle = 'rgba(245,196,81,.8)';
                for (const marker of markers) {
                  const x = Math.round(u.valToPos(marker.at, 'x', true));
                  ctx.beginPath();
                  ctx.moveTo(x, u.bbox.top);
                  ctx.lineTo(x, u.bbox.top + u.bbox.height);
                  ctx.stroke();
                }
                ctx.restore();
              },
            ],
            setCursor: [
              (u) => {
                const index = u.cursor.idx;
                if (index == null || index < 0) {
                  cursorText = '';
                  return;
                }
                const at = u.data[0][index];
                const value = u.data[1][index];
                cursorText = `${new Date(at * 1000).toLocaleString()}: ${value ?? 'gap'}`;
                tooltipLeft = u.cursor.left ?? 0;
                tooltipTop = u.cursor.top ?? 0;
              },
            ],
          },
        },
      ],
    };
  }
  onMount(() => {
    const resize = new ResizeObserver(() =>
      plot?.setSize({ width: root.clientWidth || 1, height: plot!.height }),
    );
    plot = new uPlot(options(), toSeries(points), root);
    return () => {
      resize.disconnect();
      plot?.destroy();
    };
  });
  $effect(() => {
    if (plot) plot.setData(toSeries(points));
  });
  let stats = $derived(summary(points));
  function inspect(event: KeyboardEvent) {
    if (!points.length) return;
    if (event.key === 'ArrowLeft') {
      selected = Math.max(0, selected - 1);
      event.preventDefault();
    }
    if (event.key === 'ArrowRight') {
      selected = Math.min(points.length - 1, selected + 1);
      event.preventDefault();
    }
  }
</script>

<div class="time-series">
  <div class="chart-canvas" bind:this={root} aria-hidden="true"></div>
  {#if cursorText}<span
      class="chart-tooltip"
      style={`left:${tooltipLeft}px;top:${tooltipTop}px`}>{cursorText}</span
    >{/if}
</div>
<button
  type="button"
  class="chart-inspector sr-only"
  aria-label={`${label} chart inspection`}
  onkeydown={inspect}
  ><span
    >{label}: {#if stats}minimum {stats.min}, average {stats.avg.toFixed(1)},
      maximum {stats.max}{:else}no measurements{/if}</span
  >{#if points[selected]}<span role="status"
      >Selected point: {new Date(points[selected].at * 1000).toLocaleString()}, {points[
        selected
      ].value ?? 'gap'}</span
    >{/if}{#if gaps.length}<span
      >{gaps.length} explicit data gap{gaps.length === 1 ? '' : 's'}.</span
    >{/if}</button
>
{#if markers.length}<ul class="sr-only" aria-label="Chart event annotations">
    {#each markers as marker (marker.at + marker.label)}<li>
        {#if marker.href}<a href={marker.href}
            >{new Date(marker.at * 1000).toLocaleString()}: {marker.label}</a
          >
        {:else}{new Date(marker.at * 1000).toLocaleString()}: {marker.label}{/if}
      </li>{/each}
  </ul>{/if}
