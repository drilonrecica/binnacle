<script lang="ts">
  import { clampReading, fraction } from './instrument';
  import type { Tone } from './status';

  let {
    value,
    label,
    threshold,
    tone,
    min = 0,
    max = 100,
    minLabel,
    maxLabel,
    size = 'md',
    unit = '%',
  }: {
    /** Reading between min and max; null when unknown. */
    value: number | null | undefined;
    /** Accessible name, e.g. "/var/lib/docker usage". */
    label: string;
    /** Draws a tick here; at or above it the fill turns critical. */
    threshold?: number | null;
    /**
     * Fill tone. Defaults to brass, or critical at the threshold. Pass a tone
     * only where the colour adds meaning the text already states.
     */
    tone?: Tone;
    min?: number;
    max?: number;
    /** Mono end labels; omitted labels are not drawn. */
    minLabel?: string;
    maxLabel?: string;
    size?: 'sm' | 'md';
    unit?: string;
  } = $props();

  const reading = $derived(clampReading(value, min, max));
  const limit = $derived(clampReading(threshold, min, max));
  const over = $derived(reading != null && limit != null && reading >= limit);
  const resolvedTone = $derived<Tone>(tone ?? (over ? 'critical' : 'accent'));
  const text = $derived.by(() => {
    if (reading == null) return 'No data';
    const number = Math.round(reading * 10) / 10;
    const base = unit === '%' ? `${number} percent` : `${number} ${unit}`;
    if (limit == null) return base;
    return over
      ? `${base}, at or above the threshold of ${limit}`
      : `${base}, threshold ${limit}`;
  });
</script>

<div class={`scale ${size}`}>
  <div
    class={`track ${resolvedTone}`}
    role="meter"
    aria-label={label}
    aria-valuemin={min}
    aria-valuemax={max}
    aria-valuenow={reading ?? undefined}
    aria-valuetext={text}
  >
    {#if reading != null}
      <div
        class="fill"
        style:width={`${fraction(reading, min, max) * 100}%`}
      ></div>
    {/if}
    {#if limit != null}
      <div
        class="tick"
        style:left={`${fraction(limit, min, max) * 100}%`}
        aria-hidden="true"
      ></div>
    {/if}
  </div>
  {#if minLabel || maxLabel}
    <div class="ends" aria-hidden="true">
      <span>{minLabel ?? ''}</span><span>{maxLabel ?? ''}</span>
    </div>
  {/if}
</div>

<style>
  .scale {
    display: grid;
    gap: 2px;
    width: 100%;
    min-width: 0;
  }
  .track {
    position: relative;
    width: 100%;
    height: 6px;
    border-radius: 1px;
    background: var(--line);
  }
  .sm .track {
    height: 4px;
  }
  .fill {
    height: 100%;
    border-radius: inherit;
    background: var(--fill);
    transition: width var(--motion) var(--ease-out);
  }
  .tick {
    position: absolute;
    top: -3px;
    bottom: -3px;
    width: 2px;
    margin-left: -1px;
    background: var(--fg);
  }
  .ends {
    display: flex;
    justify-content: space-between;
    color: var(--muted);
    font-family: var(--font-mono);
    font-size: var(--text-label);
    font-variant-numeric: tabular-nums;
  }
  .accent {
    --fill: var(--accent);
  }
  .ok {
    --fill: var(--ok-solid);
  }
  .warn {
    --fill: var(--warn-solid);
  }
  .critical {
    --fill: var(--critical-solid);
  }
  .info {
    --fill: var(--info-solid);
  }
  .neutral {
    --fill: var(--neutral-solid);
  }
</style>
