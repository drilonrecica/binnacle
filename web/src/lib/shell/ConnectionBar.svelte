<script lang="ts">
  import WifiOff from '@lucide/svelte/icons/wifi-off';
  import type { LiveStore } from '../live.svelte';
  import Button from '../ui/Button.svelte';
  import { formatClock } from '../ui/relative-time';

  let { live }: { live: LiveStore } = $props();
  const visible = $derived(
    live.state === 'disconnected' || live.state === 'unauthorized',
  );
</script>

{#if visible}
  <div class="bar" role="status" data-state={live.state}>
    <WifiOff aria-hidden="true" />
    <div class="text">
      <strong
        >{live.state === 'unauthorized'
          ? 'Your session ended'
          : 'Live updates disconnected'}</strong
      >
      <span>
        {live.state === 'unauthorized'
          ? 'Sign in again to resume live monitoring.'
          : live.error || 'Reconnecting automatically.'}
        {#if live.lastReceivedAt}Last update {formatClock(
            live.lastReceivedAt,
          )}.{/if}
      </span>
    </div>
    {#if live.state === 'unauthorized'}
      <Button size="sm" variant="primary" href="/login">Sign in</Button>
    {:else}
      <Button size="sm" onclick={() => live.retry()}>Retry now</Button>
    {/if}
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
    padding: var(--space-2) var(--space-5);
    border-bottom: 1px solid var(--warn-border);
    background: var(--warn-bg);
    color: var(--warn-fg);
    font-size: var(--text-sm);
  }
  .bar[data-state='unauthorized'] {
    border-color: var(--critical-border);
    background: var(--critical-bg);
    color: var(--critical-fg);
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
    color: var(--text-2);
  }
</style>
