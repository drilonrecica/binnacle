<script lang="ts">
  import type { LiveStore } from '../live.svelte';
  import { formatClock } from '../ui/relative-time';
  import { tooltip } from '../ui/tooltip';

  let { live, compact = false }: { live: LiveStore; compact?: boolean } =
    $props();

  const label = $derived(
    live.state === 'connected'
      ? 'Live'
      : live.state === 'connecting'
        ? 'Connecting'
        : live.state === 'unauthorized'
          ? 'Signed out'
          : 'Disconnected',
  );
  const detail = $derived(
    live.snapshot
      ? `Last sample ${formatClock(live.snapshot.ts)}`
      : 'Waiting for the first sample',
  );
</script>

<span
  class="status"
  data-state={live.state}
  use:tooltip={detail}
  aria-live="polite"
>
  <span class="dot" aria-hidden="true"></span>
  <span class="text">{label}</span>
  {#if !compact && live.snapshot}
    <span class="time num">{formatClock(live.snapshot.ts)}</span>
  {/if}
</span>

<style>
  .status {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    height: 28px;
    padding: 0 var(--space-2) 0 var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-full);
    background: var(--bg-subtle);
    color: var(--text-2);
    font-size: var(--text-xs);
    font-weight: 500;
    white-space: nowrap;
  }
  .dot {
    position: relative;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--neutral-solid);
  }
  [data-state='connected'] .dot {
    background: var(--ok-solid);
  }
  [data-state='connected'] .dot::after {
    content: '';
    position: absolute;
    inset: -3px;
    border-radius: 50%;
    border: 1px solid var(--ok-solid);
    opacity: 0;
    animation: ping 2.4s var(--ease-out) infinite;
  }
  [data-state='connecting'] .dot {
    background: var(--warn-solid);
  }
  [data-state='disconnected'] .dot,
  [data-state='unauthorized'] .dot {
    background: var(--critical-solid);
  }
  [data-state='connected'] .text {
    color: var(--ok-fg);
  }
  .time {
    color: var(--text-3);
  }
  @keyframes ping {
    0% {
      opacity: 0.8;
      transform: scale(0.6);
    }
    70%,
    100% {
      opacity: 0;
      transform: scale(1.9);
    }
  }
</style>
