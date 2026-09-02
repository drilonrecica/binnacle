<script lang="ts">
  import {
    normalizeStatus,
    statusLabel,
    statusTone,
    type NormalizedStatus,
  } from './status';

  let {
    status,
    label,
    size = 'md',
  }: {
    /** Raw status string from the API or an already-normalized status. */
    status: string | NormalizedStatus | null | undefined;
    /** Override the visible label. */
    label?: string;
    size?: 'sm' | 'md';
  } = $props();

  const normalized = $derived(normalizeStatus(status));
  const tone = $derived(statusTone(normalized));
  const text = $derived(label ?? statusLabel(normalized));
</script>

<span class={`pill ${tone} ${size}`} data-status={normalized}>
  <span class="dot" aria-hidden="true"></span>
  <span class="text">{text}</span>
</span>

<style>
  .pill {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    color: var(--pill-fg);
    font-size: var(--text-sm);
    font-weight: 500;
    line-height: 1;
    white-space: nowrap;
  }
  .pill.sm {
    font-size: var(--text-xs);
  }
  .dot {
    position: relative;
    width: 8px;
    height: 8px;
    flex: none;
    border-radius: 50%;
    background: var(--pill-solid);
  }
  .pill.sm .dot {
    width: 6px;
    height: 6px;
  }
  .pill[data-status='degraded'] .dot,
  .pill[data-status='down'] .dot {
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--pill-solid) 25%, transparent);
  }
  .pill[data-status='starting'] .dot::after {
    content: '';
    position: absolute;
    inset: -3px;
    border-radius: 50%;
    border: 1px solid var(--pill-solid);
    animation: pulse 1.6s var(--ease-out) infinite;
  }
  .pill[data-status='stale'] .dot,
  .pill[data-status='unknown'] .dot {
    background: transparent;
    border: 1.5px solid var(--pill-solid);
  }
  @keyframes pulse {
    0% {
      opacity: 0.9;
      transform: scale(0.8);
    }
    100% {
      opacity: 0;
      transform: scale(1.8);
    }
  }
  .ok {
    --pill-fg: var(--ok-fg);
    --pill-solid: var(--ok-solid);
  }
  .warn {
    --pill-fg: var(--warn-fg);
    --pill-solid: var(--warn-solid);
  }
  .critical {
    --pill-fg: var(--critical-fg);
    --pill-solid: var(--critical-solid);
  }
  .info {
    --pill-fg: var(--info-fg);
    --pill-solid: var(--info-solid);
  }
  .neutral {
    --pill-fg: var(--neutral-fg);
    --pill-solid: var(--neutral-solid);
  }
  .accent {
    --pill-fg: var(--accent-text);
    --pill-solid: var(--accent);
  }
</style>
