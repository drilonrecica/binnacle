<script lang="ts">
  import { describeSeries } from './instrument';
  import type { Tone } from './status';

  let {
    values,
    label,
    format = (value: number) => String(Math.round(value * 10) / 10),
    width = 120,
    height = 32,
    tone = 'accent',
    area = true,
    max,
    min = 0,
    baseline,
    threshold,
    strokeWidth = 1.5,
  }: {
    values: Array<number | null>;
    /**
     * Name for the screen-reader summary ("CPU, last hour"). The drawing is
     * hidden from assistive technology; this text carries now, min, avg and max.
     */
    label?: string;
    format?: (value: number) => string;
    width?: number;
    height?: number;
    tone?: Tone;
    area?: boolean;
    /** Fixed upper bound (e.g. 100 for percentages); auto-scales otherwise. */
    max?: number;
    min?: number;
    /** Value the baseline sits on; defaults to the bottom of the scale. */
    baseline?: number;
    /** Draws a dashed line at this value. */
    threshold?: number | null;
    strokeWidth?: number;
  } = $props();

  const geometry = $derived.by(() => {
    const finite = values.filter(
      (value): value is number => value != null && Number.isFinite(value),
    );
    const extra = [baseline, threshold ?? undefined].filter(
      (value): value is number => value != null && Number.isFinite(value),
    );
    const top = max ?? Math.max(...finite, ...extra, min + 1e-9);
    const bottom = Math.min(min, ...finite, ...extra);
    const span = top - bottom || 1;
    const pad = strokeWidth + 1;
    const innerH = height - pad * 2;
    const y = (value: number) =>
      pad + innerH - ((Math.min(top, value) - bottom) / span) * innerH;
    const base = y(baseline ?? bottom);
    const guide =
      threshold != null && Number.isFinite(threshold) ? y(threshold) : null;
    if (finite.length < 2)
      return { line: '', fill: '', last: null, base, guide };

    const step = width / (values.length - 1);
    let line = '';
    let fill = '';
    let open = false;
    let previousX = 0;
    values.forEach((value, index) => {
      const x = index * step;
      if (value == null || !Number.isFinite(value)) {
        if (open) {
          fill += ` L${previousX} ${base} Z`;
          open = false;
        }
        return;
      }
      if (!open) {
        line += `M${x} ${y(value)}`;
        fill += `M${x} ${base} L${x} ${y(value)}`;
        open = true;
      } else {
        line += ` L${x} ${y(value)}`;
        fill += ` L${x} ${y(value)}`;
      }
      previousX = x;
    });
    if (open) fill += ` L${previousX} ${base} Z`;
    let lastIndex = -1;
    for (let index = values.length - 1; index >= 0; index -= 1) {
      const value = values[index];
      if (value != null && Number.isFinite(value)) {
        lastIndex = index;
        break;
      }
    }
    const last = values[lastIndex];
    return {
      line,
      fill,
      last: last == null ? null : { x: lastIndex * step, y: y(last) },
      base,
      guide,
    };
  });

  const summary = $derived(label ? describeSeries(label, values, format) : '');
</script>

<span class="spark-wrap">
  <svg
    class={`sparkline ${tone}`}
    viewBox={`0 0 ${width} ${height}`}
    {width}
    {height}
    preserveAspectRatio="none"
    aria-hidden="true"
    focusable="false"
  >
    <line
      class="baseline"
      x1="0"
      y1={geometry.base}
      x2={width}
      y2={geometry.base}
    />
    {#if geometry.guide != null}
      <line
        class="threshold"
        x1="0"
        y1={geometry.guide}
        x2={width}
        y2={geometry.guide}
      />
    {/if}
    {#if geometry.line}
      {#if area}<path class="area" d={geometry.fill} />{/if}
      <path class="line" d={geometry.line} stroke-width={strokeWidth} />
      {#if geometry.last}
        <circle
          class="halo"
          cx={geometry.last.x}
          cy={geometry.last.y}
          r={strokeWidth + 2.5}
        />
        <circle
          class="dot"
          cx={geometry.last.x}
          cy={geometry.last.y}
          r={strokeWidth + 1}
        />
      {/if}
    {/if}
  </svg>
  {#if summary}<span class="sr-only">{summary}</span>{/if}
</span>

<style>
  .spark-wrap {
    display: inline-flex;
  }
  .sparkline {
    display: block;
    overflow: visible;
    color: var(--spark);
  }
  .line {
    fill: none;
    stroke: currentColor;
    stroke-linejoin: round;
    stroke-linecap: round;
    vector-effect: non-scaling-stroke;
  }
  .area {
    fill: currentColor;
    opacity: var(--chart-area);
  }
  .baseline {
    stroke: var(--line-strong);
    stroke-width: 1;
    vector-effect: non-scaling-stroke;
  }
  .threshold {
    stroke: var(--critical-solid);
    stroke-width: 1;
    stroke-dasharray: 3 3;
    vector-effect: non-scaling-stroke;
  }
  .dot {
    fill: currentColor;
  }
  .halo {
    fill: var(--panel);
    stroke: currentColor;
    stroke-width: 1;
    vector-effect: non-scaling-stroke;
  }
  .accent {
    --spark: var(--chart-1);
  }
  .ok {
    --spark: var(--ok-solid);
  }
  .warn {
    --spark: var(--warn-solid);
  }
  .critical {
    --spark: var(--critical-solid);
  }
  .info {
    --spark: var(--info-solid);
  }
  .neutral {
    --spark: var(--neutral-solid);
  }
</style>
