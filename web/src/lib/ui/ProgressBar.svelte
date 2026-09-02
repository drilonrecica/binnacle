<script lang="ts">
  import { utilizationTone, type Tone } from './status';

  let {
    value,
    label,
    tone,
    warn = 80,
    critical = 95,
    size = 'md',
  }: {
    /** Percentage 0–100, or null when unknown. */
    value: number | null | undefined;
    /** Accessible name, e.g. "Host memory". */
    label: string;
    tone?: Tone;
    warn?: number;
    critical?: number;
    size?: 'sm' | 'md';
  } = $props();

  const clamped = $derived(
    value == null || !Number.isFinite(value)
      ? null
      : Math.min(100, Math.max(0, value)),
  );
  const resolvedTone = $derived(
    tone ?? utilizationTone(clamped, warn, critical),
  );
</script>

<div
  class={`bar ${resolvedTone} ${size}`}
  role="progressbar"
  aria-label={label}
  aria-valuemin="0"
  aria-valuemax="100"
  aria-valuenow={clamped ?? undefined}
  aria-valuetext={clamped == null ? 'Unavailable' : `${Math.round(clamped)}%`}
>
  {#if clamped != null}
    <div class="fill" style:width={`${clamped}%`}></div>
  {/if}
</div>

<style>
  .bar {
    position: relative;
    width: 100%;
    height: 6px;
    overflow: hidden;
    border-radius: var(--radius-full);
    background: var(--surface-3);
  }
  .bar.sm {
    height: 4px;
  }
  .fill {
    height: 100%;
    border-radius: inherit;
    background: var(--fill);
    transition: width var(--motion) var(--ease);
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
  .accent {
    --fill: var(--accent);
  }
  .neutral {
    --fill: var(--neutral-solid);
  }
</style>
