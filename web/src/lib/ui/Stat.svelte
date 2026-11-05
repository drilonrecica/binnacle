<script lang="ts">
  import type { Snippet } from 'svelte';
  import Scale from './Scale.svelte';
  import Sparkline from './Sparkline.svelte';
  import { type Tone } from './status';

  let {
    label,
    value,
    unit = '',
    secondary,
    percent,
    threshold,
    tone,
    sparkline,
    sparklineMax,
    sparklineLabel,
    href,
    icon,
    children,
  }: {
    label: string;
    value: string;
    unit?: string;
    /** Small line under the value, e.g. "of 8 GiB · 2.1 GiB free". */
    secondary?: string;
    /** Utilization percentage; renders a linear scale under the value. */
    percent?: number | null;
    /** Threshold tick on the scale and sparkline. */
    threshold?: number | null;
    /** Colours the value; only for states the text also states. */
    tone?: Tone;
    sparkline?: Array<number | null>;
    sparklineMax?: number;
    /** Names the sparkline's screen-reader summary, e.g. "last hour". */
    sparklineLabel?: string;
    href?: string;
    icon?: Snippet;
    children?: Snippet;
  } = $props();
</script>

<svelte:element
  this={href ? 'a' : 'div'}
  {href}
  class={`stat ${tone ?? ''}`}
  class:linked={Boolean(href)}
>
  <div class="top">
    <span class="label">
      {#if icon}<span class="icon" aria-hidden="true">{@render icon()}</span
        >{/if}
      {label}
    </span>
    {#if sparkline}
      <Sparkline
        values={sparkline}
        max={sparklineMax}
        {threshold}
        label={`${label}${sparklineLabel ? `, ${sparklineLabel}` : ''}`}
        width={96}
        height={28}
      />
    {/if}
  </div>
  <div class="reading">
    <span class="value">{value}</span>
    {#if unit}<span class="unit">{unit}</span>{/if}
  </div>
  {#if percent !== undefined}
    <Scale
      value={percent}
      label={`${label} utilization`}
      {threshold}
      size="sm"
    />
  {/if}
  {#if secondary}<p class="secondary">{secondary}</p>{/if}
  {@render children?.()}
</svelte:element>

<style>
  .stat {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
    padding: var(--card-pad-wide);
    color: inherit;
    text-decoration: none;
  }
  .stat.linked:hover {
    background: var(--panel-2);
    text-decoration: none;
  }
  .top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    min-height: 28px;
  }
  .label {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--muted);
    font-size: var(--text-label);
    font-weight: var(--weight-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }
  .icon {
    display: inline-flex;
  }
  .icon :global(svg) {
    width: 14px;
    height: 14px;
  }
  .reading {
    display: flex;
    align-items: baseline;
    gap: var(--space-1);
  }
  .value {
    font-family: var(--font-display);
    font-size: var(--text-gauge);
    font-weight: var(--weight-semibold);
    font-variant-numeric: tabular-nums;
    letter-spacing: var(--tracking-display);
    line-height: 1;
  }
  .unit {
    color: var(--muted);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }
  .secondary {
    overflow: hidden;
    color: var(--muted);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .critical .value {
    color: var(--critical-fg);
  }
  .warn .value {
    color: var(--warn-fg);
  }
  .ok .value {
    color: var(--ok-fg);
  }
</style>
