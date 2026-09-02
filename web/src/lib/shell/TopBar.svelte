<script lang="ts">
  import Search from '@lucide/svelte/icons/search';
  import type { LiveStore } from '../live.svelte';
  import { viewport } from '../ui/media.svelte';
  import Kbd from '../ui/Kbd.svelte';
  import { shell } from './shell-state.svelte';
  import ConnectionStatus from './ConnectionStatus.svelte';

  let { live }: { live: LiveStore } = $props();

  const isMac =
    typeof navigator !== 'undefined' &&
    /Mac|iPhone|iPad/.test(navigator.platform);
  const degradedCollectors = $derived(
    live.snapshot
      ? Object.entries(live.snapshot.collectors).filter(
          ([, collector]) => collector.state !== 'healthy',
        )
      : [],
  );
</script>

<div class="topbar">
  {#if viewport.isMobile}
    <a class="brand" href="/overview" aria-label="Binnacle overview">
      <img
        class="mark dark"
        src="/brand/binnacle-mark-dark.png"
        alt=""
        width="24"
        height="24"
      />
      <img
        class="mark light"
        src="/brand/binnacle-mark.png"
        alt=""
        width="24"
        height="24"
      />
    </a>
  {/if}
  <div class="spacer"></div>

  {#if degradedCollectors.length}
    <a class="chip warn" href="/host?tab=collectors">
      <span class="chip-dot" aria-hidden="true"></span>
      {degradedCollectors.length === 1
        ? `${degradedCollectors[0][0]} collector ${degradedCollectors[0][1].state}`
        : `${degradedCollectors.length} collectors degraded`}
    </a>
  {/if}

  {#if viewport.isMobile}
    <ConnectionStatus {live} compact />
  {/if}

  <button
    type="button"
    class="search"
    aria-label="Search"
    onclick={() => (shell.paletteOpen = true)}
    aria-keyshortcuts={isMac ? 'Meta+K' : 'Control+K'}
  >
    <Search aria-hidden="true" />
    <span class="search-label">Search</span>
    <span class="keys" aria-hidden="true"
      ><Kbd>{isMac ? '⌘' : 'Ctrl'}</Kbd><Kbd>K</Kbd></span
    >
  </button>
</div>

<style>
  .topbar {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    height: var(--topbar-h);
    padding: 0 var(--space-5);
    border-bottom: 1px solid var(--border);
    background: color-mix(in srgb, var(--bg) 85%, transparent);
    backdrop-filter: blur(8px);
  }
  .brand {
    display: inline-flex;
  }
  .mark.light {
    display: none;
  }
  :global(html[data-theme='light']) .mark.light {
    display: block;
  }
  :global(html[data-theme='light']) .mark.dark {
    display: none;
  }
  .spacer {
    flex: 1;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    height: 28px;
    padding: 0 var(--space-3);
    border: 1px solid var(--warn-border);
    border-radius: var(--radius-full);
    background: var(--warn-bg);
    color: var(--warn-fg);
    font-size: var(--text-xs);
    font-weight: 500;
    white-space: nowrap;
    text-decoration: none;
  }
  .chip-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--warn-solid);
  }
  .search {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    height: 32px;
    padding: 0 var(--space-2) 0 var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text-3);
    font-size: var(--text-sm);
  }
  .search:hover {
    color: var(--text);
    border-color: var(--n-600);
  }
  .search :global(svg) {
    width: 15px;
    height: 15px;
  }
  .keys {
    display: inline-flex;
    gap: 3px;
    margin-left: var(--space-2);
  }
  @media (max-width: 899px) {
    .topbar {
      padding: 0 var(--space-4);
    }
    .search {
      width: 32px;
      padding: 0;
      justify-content: center;
    }
    .search-label,
    .keys {
      display: none;
    }
    .chip {
      display: none;
    }
  }
</style>
