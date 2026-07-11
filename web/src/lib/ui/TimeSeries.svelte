<script lang="ts">
  import { onMount } from 'svelte'; import uPlot from 'uplot'; import 'uplot/dist/uPlot.min.css'; import { summary, toSeries, type Point } from '../chart';
  let { points, label, variant = 'line' }: { points: Point[]; label: string; variant?: 'line'|'area'|'sparkline' } = $props(); let root: HTMLDivElement; let plot: uPlot | undefined;
  function options(): uPlot.Options { return { width: root.clientWidth || 1, height: variant === 'sparkline' ? 48 : 180, series: [{}, { label, stroke: 'var(--chart-1)', fill: variant === 'area' ? 'rgb(120 220 232 / .2)' : undefined }], axes: variant === 'sparkline' ? [] : [{}, {}] }; }
  onMount(() => { const resize = new ResizeObserver(() => plot?.setSize({ width: root.clientWidth || 1, height: plot!.height })); plot = new uPlot(options(), toSeries(points), root); return () => { resize.disconnect(); plot?.destroy(); }; });
  $effect(() => { if (plot) plot.setData(toSeries(points)); });
  let stats = $derived(summary(points));
</script>
<div bind:this={root} aria-hidden="true"></div><p class="sr-only">{label}: {#if stats}minimum {stats.min}, average {stats.avg.toFixed(1)}, maximum {stats.max}{:else}no measurements{/if}</p>
