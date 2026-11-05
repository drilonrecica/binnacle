<script lang="ts">
  import type { Tone } from './status';

  // Progress of a task (a deletion, an export). Readings such as disk usage
  // are not progress: use Scale, which is a meter.
  let {
    value,
    label,
    tone = 'accent',
    size = 'md',
  }: {
    /** Percentage 0–100, or null when unknown. */
    value: number | null | undefined;
    /** Accessible name, e.g. "Deletion progress". */
    label: string;
    /** `critical` for a failed task; the caller states the failure in text. */
    tone?: Tone;
    size?: 'sm' | 'md';
  } = $props();

  const clamped = $derived(
    value == null || !Number.isFinite(value)
      ? null
      : Math.min(100, Math.max(0, value)),
  );
</script>

<div
  class={`bar ${tone} ${size}`}
  role="progressbar"
  aria-label={label}
  aria-valuemin="0"
  aria-valuemax="100"
  aria-valuenow={clamped ?? undefined}
  aria-valuetext={clamped == null ? 'Unknown' : `${Math.round(clamped)}%`}
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
    border-radius: 1px;
    background: var(--line);
  }
  .bar.sm {
    height: 4px;
  }
  .fill {
    height: 100%;
    background: var(--fill);
    transition: width var(--motion) var(--ease-out);
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
