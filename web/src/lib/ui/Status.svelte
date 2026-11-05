<script lang="ts">
  import StatusMarker from './StatusMarker.svelte';
  import {
    normalizeStatus,
    statusLabel,
    statusTone,
    type NormalizedStatus,
    type Tone,
  } from './status';

  let {
    status,
    tone,
    label,
    size = 'md',
    hideLabel = false,
  }: {
    /** Raw status string from the API or an already-normalized status. */
    status?: string | NormalizedStatus | null;
    /** Sets the tone directly instead of deriving it from `status`. */
    tone?: Tone;
    /** Overrides the word shown beside the marker. */
    label?: string;
    size?: 'sm' | 'md';
    /** Keeps the word for screen readers only, for dense cells. */
    hideLabel?: boolean;
  } = $props();

  const normalized = $derived(normalizeStatus(status));
  const resolvedTone = $derived(tone ?? statusTone(normalized));
  const text = $derived(label ?? statusLabel(normalized));
</script>

<span
  class={`status ${resolvedTone} ${size}`}
  data-status={status == null && tone ? undefined : normalized}
  data-tone={resolvedTone}
>
  <StatusMarker tone={resolvedTone} size={size === 'sm' ? 7 : 8} />
  <span class="text" class:sr-only={hideLabel}>{text}</span>
</span>

<style>
  .status {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    color: var(--status-fg);
    font-size: var(--text-body);
    font-weight: var(--weight-medium);
    line-height: 1;
    white-space: nowrap;
  }
  .status.sm {
    gap: 0.3rem;
    font-size: var(--text-meta);
  }
  .ok {
    --status-fg: var(--ok-fg);
  }
  .warn {
    --status-fg: var(--warn-fg);
  }
  .critical {
    --status-fg: var(--critical-fg);
  }
  .info {
    --status-fg: var(--info-fg);
  }
  .neutral {
    --status-fg: var(--neutral-fg);
  }
  .accent {
    --status-fg: var(--accent-text);
  }
</style>
