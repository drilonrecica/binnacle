<script lang="ts">
  import Search from '@lucide/svelte/icons/search';
  import type { LiveStore } from '../live.svelte';
  import { homePath } from '../router';
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
  const collectorSummary = $derived.by(() => {
    const [first] = degradedCollectors;
    if (!first) return null;
    if (degradedCollectors.length === 1)
      return {
        long: `${first[0]} collector ${first[1].state}`,
        short: `${first[0]} ${first[1].state}`,
      };
    return {
      long: `${degradedCollectors.length} collectors degraded`,
      short: `${degradedCollectors.length} degraded`,
    };
  });
</script>

<div class="topbar">
  {#if viewport.isMobile}
    <a class="brand" href={homePath} aria-label="Binnacle, Headroom">
      <img
        class="mark dark"
        src="/brand/binnacle-mark.svg"
        alt=""
        width="24"
        height="24"
      />
      <img
        class="mark light"
        src="/brand/binnacle-mark-on-light.svg"
        alt=""
        width="24"
        height="24"
      />
    </a>
  {/if}
  <div class="spacer"></div>

  {#if collectorSummary}
    <!-- Kept on phones in a compact form: a degraded collector means the
         numbers on every page may be incomplete. -->
    <a
      class="chip warn"
      href="/server?tab=collectors"
      aria-label={collectorSummary.long}
    >
      <span class="chip-dot" aria-hidden="true"></span>
      <span class="chip-long" aria-hidden="true">{collectorSummary.long}</span>
      <span class="chip-short" aria-hidden="true">{collectorSummary.short}</span
      >
    </a>
  {/if}

  <ConnectionStatus {live} compact={viewport.isMobile} />

  <button
    type="button"
    class="search"
    onclick={() => (shell.paletteOpen = true)}
    aria-keyshortcuts={isMac ? 'Meta+K' : 'Control+K'}
  >
    <Search aria-hidden="true" />
    <!-- The label names the button from its content (no aria-label, so the
         visible shortcut hint cannot contradict the accessible name). At phone
         width it is visually hidden rather than removed so the name survives. -->
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
    padding: 0 var(--space-4);
    border-bottom: 1px solid var(--line);
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
    border-radius: var(--radius-control);
    background: var(--warn-bg);
    color: var(--warn-fg);
    font-size: var(--text-mono);
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
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-control);
    background: var(--panel);
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .search:hover {
    color: var(--fg);
    border-color: var(--muted);
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
    .search-label {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
    }
    .keys {
      display: none;
    }
    .chip {
      padding: 0 var(--space-2);
    }
    .chip-long {
      display: none;
    }
  }
  @media (min-width: 900px) {
    .chip-short {
      display: none;
    }
  }
</style>
