<script lang="ts">
  import WifiOff from '@lucide/svelte/icons/wifi-off';
  import type { LiveStore } from '../live.svelte';
  import Button from '../ui/Button.svelte';
  import { formatClock } from '../ui/relative-time';

  let { live }: { live: LiveStore } = $props();
  // A lost session never shows here: the app ends it and sends the user to
  // sign in with a `next` back to this page (UI-5). This bar is not a live
  // region; the connection status in the top bar announces the state once.
  const visible = $derived(live.state === 'disconnected');
</script>

{#if visible}
  <div class="bar" data-state={live.state}>
    <WifiOff aria-hidden="true" />
    <div class="text">
      <strong>Live updates disconnected</strong>
      <span>
        {live.error || 'Reconnecting automatically.'}
        {#if live.lastReceivedAt}Last update {formatClock(
            live.lastReceivedAt,
          )}.{/if}
      </span>
    </div>
    <Button size="sm" onclick={() => live.retry()}>Retry now</Button>
  </div>
{/if}

<style>
  .bar {
    position: sticky;
    top: 0;
    z-index: 30;
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-4);
    border-bottom: 1px solid var(--warn-border);
    background: var(--warn-bg);
    color: var(--warn-fg);
    font-size: var(--text-meta);
  }
  .bar :global(svg) {
    width: 16px;
    height: 16px;
    flex: none;
  }
  .text {
    display: flex;
    flex: 1;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-2);
    min-width: 0;
  }
  .text span {
    color: var(--muted);
  }
</style>
