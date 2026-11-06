<script lang="ts">
  import { missing } from '../i18n';
  import type { Snippet } from 'svelte';
  import StatusMarker from './StatusMarker.svelte';
  import { arcPath, arcPoint, clampReading, fraction } from './instrument';

  let {
    value,
    label,
    sublabel,
    threshold,
    warn,
    valueText,
    unit = '%',
    size = 'md',
    caption,
    trend,
    trendTone = 'accent',
    children,
  }: {
    /** Reading on a true 0..100 scale; null when unknown. */
    value: number | null | undefined;
    /** Instrument name, e.g. "CPU" or "/var/lib/docker". */
    label: string;
    /** Second line under the label in mono, e.g. a mount point. */
    sublabel?: string;
    /** Above this the fill turns critical and a tick marks the line. */
    threshold?: number | null;
    /** Above this (and below `threshold`) the fill takes the warn tone. */
    warn?: number | null;
    /** Overrides the displayed number, e.g. "73.4". Defaults to the rounded value. */
    valueText?: string;
    unit?: string;
    /** `lg` is for the first instrument on a page. */
    size?: 'md' | 'lg';
    /** One line under the value, e.g. "6.1 of 8 GiB". */
    caption?: string;
    /** Brass trend text, e.g. "full in 6 days". */
    trend?: string;
    /** Brass when the trend carries a date, critical when it is close, muted otherwise. */
    trendTone?: 'accent' | 'critical' | 'muted';
    children?: Snippet;
  } = $props();

  // Geometry on a 120 x 68 view box; the ring stroke matches the compass
  // mark's proportion (1.5 on a 10.5 radius).
  const cx = 60;
  const cy = 60;
  const radius = 50;
  const stroke = 7;

  const reading = $derived(clampReading(value));
  const limit = $derived(clampReading(threshold));
  const over = $derived(reading != null && limit != null && reading >= limit);
  const warnLimit = $derived(clampReading(warn));
  const warning = $derived(
    !over && reading != null && warnLimit != null && reading >= warnLimit,
  );
  const shown = $derived(
    reading == null ? missing : (valueText ?? String(Math.round(reading))),
  );
  const fillPath = $derived(
    reading == null || reading <= 0
      ? null
      : arcPath(cx, cy, radius, 0, fraction(reading)),
  );
  function tickAt(value: number | null) {
    if (value == null) return null;
    const at = fraction(value);
    return {
      inner: arcPoint(cx, cy, radius - stroke / 2 - 3, at),
      outer: arcPoint(cx, cy, radius + stroke / 2 + 3, at),
    };
  }
  const tick = $derived(tickAt(limit));
  const warnTick = $derived(tickAt(warnLimit));
  const needleAngle = $derived(reading == null ? 0 : fraction(reading) * 180);
  // A reading in bytes still says its percentage, the scale the arc uses.
  const spoken = $derived(
    unit === '%'
      ? `${shown} percent`
      : unit
        ? `${shown} ${unit}`
        : `${shown}, ${Math.round(reading ?? 0)} percent`,
  );
  const valueTextForAt = $derived(
    reading == null
      ? 'No data'
      : `${spoken}${
          limit != null
            ? over
              ? `, at or above the ${Math.round(limit)} percent threshold`
              : `, threshold ${Math.round(limit)} percent`
            : ''
        }`,
  );
</script>

<div
  class={`gauge ${size}`}
  class:over
  class:warning
  class:unknown={reading == null}
  role={reading == null ? 'img' : 'meter'}
  aria-label={reading == null ? `${label}: no data` : label}
  aria-valuemin={reading == null ? undefined : 0}
  aria-valuemax={reading == null ? undefined : 100}
  aria-valuenow={reading ?? undefined}
  aria-valuetext={reading == null ? undefined : valueTextForAt}
>
  <svg viewBox="0 0 120 68" aria-hidden="true" focusable="false">
    <path
      class="track"
      d={arcPath(cx, cy, radius, 0, 1)}
      stroke-width={stroke}
    />
    {#if fillPath}
      <path class="fill" d={fillPath} stroke-width={stroke} />
    {/if}
    {#if warnTick}
      <line
        class="tick warn-tick"
        x1={warnTick.inner.x}
        y1={warnTick.inner.y}
        x2={warnTick.outer.x}
        y2={warnTick.outer.y}
      />
    {/if}
    {#if tick}
      <line
        class="tick"
        x1={tick.inner.x}
        y1={tick.inner.y}
        x2={tick.outer.x}
        y2={tick.outer.y}
      />
    {/if}
    {#if reading != null}
      <!-- Compass-needle shape drawn pointing at 0 and rotated to the reading. -->
      <g class="needle" style:transform={`rotate(${needleAngle}deg)`}>
        <path
          d={`M${cx - radius + 9} ${cy} L${cx} ${cy - 2.2} L${cx} ${cy + 2.2} Z`}
        />
      </g>
    {/if}
    <circle class="hub" {cx} {cy} r="3.2" />
  </svg>
  <div class="readout">
    <span class="label">{label}</span>
    {#if sublabel}<span class="sublabel">{sublabel}</span>{/if}
    <span class="value">
      <span class="number">{shown}</span>{#if reading != null && unit}<span
          class="unit">{unit}</span
        >{/if}
    </span>
    {#if over && limit != null}
      <span class="over-word">
        <StatusMarker tone="critical" size={7} />Above {Math.round(limit)}%
      </span>
    {:else if warning && warnLimit != null}
      <span class="over-word warn-word">
        <StatusMarker tone="warn" size={7} />Above {Math.round(warnLimit)}%
      </span>
    {/if}
    {#if caption}<span class="caption">{caption}</span>{/if}
    {#if trend}<span class={`trend ${trendTone}`}>{trend}</span>{/if}
    {@render children?.()}
  </div>
</div>

<style>
  .gauge {
    display: grid;
    justify-items: center;
    gap: var(--space-1);
    min-width: 0;
    text-align: center;
  }
  svg {
    width: 100%;
    max-width: 180px;
    overflow: visible;
  }
  .lg svg {
    max-width: 220px;
  }
  .track,
  .fill {
    fill: none;
    stroke-linecap: butt;
  }
  .track {
    stroke: var(--line);
  }
  .fill {
    stroke: var(--accent);
    transition: stroke var(--motion) var(--ease-out);
  }
  .warning .fill {
    stroke: var(--warn-solid);
  }
  .over .fill {
    stroke: var(--critical-solid);
  }
  .tick {
    stroke: var(--muted);
    stroke-width: 1.5;
  }
  .needle {
    fill: var(--fg);
    transform-box: view-box;
    transform-origin: 60px 60px;
    transition: transform var(--motion-needle) var(--ease-out);
  }
  .hub {
    fill: var(--fg);
  }
  .unknown .hub {
    fill: var(--muted);
  }
  .readout {
    display: grid;
    justify-items: center;
    gap: 2px;
    margin-top: calc(var(--space-2) * -1);
  }
  .label {
    color: var(--muted);
    font-size: var(--text-label);
    font-weight: var(--weight-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }
  .sublabel {
    max-width: 100%;
    overflow: hidden;
    color: var(--muted);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .value {
    display: inline-flex;
    align-items: baseline;
    gap: 2px;
    font-family: var(--font-display);
    font-weight: var(--weight-semibold);
    font-variant-numeric: tabular-nums;
    line-height: 1;
    letter-spacing: var(--tracking-display);
  }
  .number {
    font-size: var(--text-gauge);
  }
  .lg .number {
    font-size: var(--text-gauge-lg);
  }
  .unknown .number {
    color: var(--muted);
  }
  .unit {
    color: var(--muted);
    font-size: var(--text-h2);
  }
  .over-word {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    color: var(--critical-fg);
    font-size: var(--text-meta);
    font-weight: var(--weight-medium);
  }
  .caption {
    color: var(--muted);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    font-variant-numeric: tabular-nums;
  }
  .warn-word {
    color: var(--warn-fg);
  }
  .trend {
    color: var(--accent-text);
    font-size: var(--text-meta);
    font-weight: var(--weight-medium);
  }
  .trend.critical {
    color: var(--critical-fg);
  }
  .trend.muted {
    color: var(--muted);
  }
</style>
