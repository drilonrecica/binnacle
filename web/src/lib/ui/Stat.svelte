<script lang="ts">
  import type { Snippet } from 'svelte';
  import ProgressBar from './ProgressBar.svelte';
  import Sparkline from './Sparkline.svelte';
  import { utilizationTone, type Tone } from './status';

  let {
    label,
    value,
    unit = '',
    secondary,
    percent,
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
    /** Utilization percentage; renders a progress bar and drives the tone. */
    percent?: number | null;
    tone?: Tone;
    sparkline?: Array<number | null>;
    sparklineMax?: number;
    /** Text alternative for the sparkline, e.g. "last hour". */
    sparklineLabel?: string;
    href?: string;
    icon?: Snippet;
    children?: Snippet;
  } = $props();

  const resolvedTone = $derived(
    tone ?? (percent === undefined ? 'accent' : utilizationTone(percent)),
  );
</script>

<svelte:element
  this={href ? 'a' : 'div'}
  {href}
  class={`stat ${resolvedTone}`}
  class:linked={Boolean(href)}
>
  <div class="top">
    <span class="label">
      {#if icon}<span class="icon">{@render icon()}</span>{/if}
      {label}
    </span>
    {#if sparkline}
      <span class="spark" title={sparklineLabel}>
        <Sparkline
          values={sparkline}
          max={sparklineMax}
          tone={resolvedTone}
          width={96}
          height={28}
        />
        {#if sparklineLabel}<span class="sr-only">{sparklineLabel}</span>{/if}
      </span>
    {/if}
  </div>
  <div class="reading">
    <span class="value num">{value}</span>
    {#if unit}<span class="unit">{unit}</span>{/if}
  </div>
  {#if percent !== undefined}
    <ProgressBar
      value={percent}
      label={`${label} utilization`}
      tone={resolvedTone}
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
    padding: var(--space-4);
    color: inherit;
    text-decoration: none;
  }
  .stat.linked:hover {
    background: var(--surface-2);
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
    color: var(--text-2);
    font-size: var(--text-sm);
    font-weight: 500;
  }
  .icon {
    display: inline-flex;
    color: var(--text-3);
  }
  .icon :global(svg) {
    width: 15px;
    height: 15px;
  }
  .spark {
    display: inline-flex;
    flex: none;
  }
  .reading {
    display: flex;
    align-items: baseline;
    gap: var(--space-1);
  }
  .value {
    font-size: var(--text-num);
    font-weight: 500;
    letter-spacing: -0.02em;
    line-height: 1;
  }
  .unit {
    color: var(--text-3);
    font-size: var(--text-sm);
    font-family: var(--font-mono);
  }
  .secondary {
    color: var(--text-3);
    font-size: var(--text-xs);
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .critical .value {
    color: var(--critical-fg);
  }
  .warn .value {
    color: var(--warn-fg);
  }
</style>
