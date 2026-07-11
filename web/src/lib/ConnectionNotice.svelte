<script lang="ts">
  import type { LiveStore } from './live.svelte';
  let { live }: { live: LiveStore } = $props();
</script>

{#if live.state === 'disconnected' || live.state === 'unauthorized'}
  <aside class="connection-notice" role="status">
    <strong
      >{live.state === 'unauthorized'
        ? 'Session expired'
        : 'Live updates disconnected'}</strong
    >
    <p>
      {live.error || 'Current values are unavailable.'}{#if live.snapshot}
        Last known values remain visible and are marked stale.{/if}
    </p>
    {#if live.lastReceivedAt}<p>
        Last update: <time datetime={live.lastReceivedAt}
          >{new Date(live.lastReceivedAt).toLocaleString()}</time
        >
      </p>{/if}
    {#if live.state !== 'unauthorized'}<button
        type="button"
        onclick={() => live.retry()}>Retry now</button
      >{/if}
  </aside>
{/if}
