<script lang="ts">
  import type { LiveStore } from '../live.svelte';
  import { tooltip } from '../ui/tooltip';
  import { connectionView } from './connection';

  let { live, compact = false }: { live: LiveStore; compact?: boolean } =
    $props();

  // A local ticker only decides when quiet data turns stale and how old it
  // is; nothing it drives sits inside the live region.
  let now = $state(Date.now());
  $effect(() => {
    const timer = window.setInterval(() => (now = Date.now()), 2_000);
    return () => window.clearInterval(timer);
  });

  const view = $derived(connectionView(live.state, live.lastReceivedAt, now));
</script>

<!-- The one status region for the connection (UI-3): only the state word is
     announced; the "since" time and the ticking age stay outside it. -->
<span
  class="status"
  data-tone={view.tone}
  data-testid="connection-status"
  use:tooltip={view.detail}
>
  <span class="dot" aria-hidden="true"></span>
  <span class="text" role="status">{view.label}</span>
  {#if view.since}
    <span class="since num">{compact ? view.age : view.since}</span>
  {/if}
</span>

<style>
  .status {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    height: 28px;
    padding: 0 var(--space-3);
    border: 1px solid var(--line);
    border-radius: var(--radius-control);
    background: var(--bg);
    color: var(--muted);
    font-size: var(--text-mono);
    font-weight: 500;
    white-space: nowrap;
  }
  .dot {
    position: relative;
    flex: none;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--neutral-solid);
  }
  /* Shapes as well as colour: a ring for warn, a square for critical. */
  [data-tone='ok'] .dot {
    background: var(--ok-solid);
  }
  [data-tone='ok'] .dot::after {
    content: '';
    position: absolute;
    inset: -3px;
    border-radius: 50%;
    border: 1px solid var(--ok-solid);
    opacity: 0;
    animation: ping 2.4s var(--ease-out) infinite;
  }
  [data-tone='warn'] .dot {
    background: transparent;
    border: 2px solid var(--warn-solid);
  }
  [data-tone='critical'] .dot {
    border-radius: 1px;
    background: var(--critical-solid);
  }
  [data-tone='ok'] .text {
    color: var(--ok-fg);
  }
  [data-tone='warn'] .text {
    color: var(--warn-fg);
  }
  [data-tone='critical'] .text {
    color: var(--critical-fg);
  }
  .since {
    color: var(--muted);
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
  @media (prefers-reduced-motion: reduce) {
    [data-tone='ok'] .dot::after {
      animation: none;
    }
  }
</style>
