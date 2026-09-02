<script lang="ts">
  import type { Tone } from './status';

  let {
    values,
    width = 120,
    height = 32,
    tone = 'accent',
    area = true,
    max,
    min = 0,
    strokeWidth = 1.5,
  }: {
    values: Array<number | null>;
    width?: number;
    height?: number;
    tone?: Tone;
    area?: boolean;
    /** Fixed upper bound (e.g. 100 for percentages); auto-scales otherwise. */
    max?: number;
    min?: number;
    strokeWidth?: number;
  } = $props();

  const geometry = $derived.by(() => {
    const finite = values.filter(
      (value): value is number => value != null && Number.isFinite(value),
    );
    if (finite.length < 2) return null;
    const top = max ?? Math.max(...finite, min + 1e-9);
    const bottom = Math.min(min, ...finite);
    const span = top - bottom || 1;
    const pad = strokeWidth;
    const innerH = height - pad * 2;
    const step = values.length > 1 ? width / (values.length - 1) : width;
    const y = (value: number) =>
      pad + innerH - ((value - bottom) / span) * innerH;
    let line = '';
    let fill = '';
    let open = false;
    let segmentStart = 0;
    values.forEach((value, index) => {
      const x = index * step;
      if (value == null || !Number.isFinite(value)) {
        if (open) {
          fill += ` L${x - step} ${height} Z`;
          open = false;
        }
        return;
      }
      if (!open) {
        line += `M${x} ${y(value)}`;
        fill += `M${x} ${height} L${x} ${y(value)}`;
        segmentStart = x;
        open = true;
      } else {
        line += ` L${x} ${y(value)}`;
        fill += ` L${x} ${y(value)}`;
      }
    });
    if (open) fill += ` L${(values.length - 1) * step} ${height} Z`;
    void segmentStart;
    const lastIndex =
      values.length - 1 - [...values].reverse().findIndex((v) => v != null);
    const last = values[lastIndex];
    return {
      line,
      fill,
      last: last == null ? null : { x: lastIndex * step, y: y(last) },
    };
  });
</script>

<svg
  class={`sparkline ${tone}`}
  viewBox={`0 0 ${width} ${height}`}
  {width}
  {height}
  preserveAspectRatio="none"
  aria-hidden="true"
>
  {#if geometry}
    {#if area}<path class="area" d={geometry.fill} />{/if}
    <path class="line" d={geometry.line} stroke-width={strokeWidth} />
    {#if geometry.last}
      <circle
        class="dot"
        cx={geometry.last.x}
        cy={geometry.last.y}
        r={strokeWidth + 0.5}
      />
    {/if}
  {:else}
    <line class="flat" x1="0" y1={height / 2} x2={width} y2={height / 2} />
  {/if}
</svg>

<style>
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
  .dot {
    fill: currentColor;
  }
  .flat {
    stroke: var(--border-strong);
    stroke-dasharray: 2 3;
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
