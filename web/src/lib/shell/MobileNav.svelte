<script lang="ts">
  import Ellipsis from '@lucide/svelte/icons/ellipsis';
  import LogOut from '@lucide/svelte/icons/log-out';
  import Moon from '@lucide/svelte/icons/moon';
  import Sun from '@lucide/svelte/icons/sun';
  import type { SessionInfo } from '../auth';
  import { logout } from '../auth';
  import { router } from '../router.svelte';
  import { prefs } from '../preferences.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import { toasts } from '../ui/toast.svelte';
  import { errorMessage } from '../api/client';
  import { mobilePrimary, navigation } from './navigation';
  import { shell } from './shell-state.svelte';

  let { session, onlogout }: { session: SessionInfo; onlogout: () => void } =
    $props();

  let moreOpen = $state(false);
  const primary = $derived(
    navigation.filter((item) => mobilePrimary.includes(item.route)),
  );
  const secondary = $derived(
    navigation.filter((item) => !mobilePrimary.includes(item.route)),
  );
  const moreActive = $derived(
    secondary.some((item) => item.matches.includes(router.name ?? 'overview')),
  );
  const nextTheme = $derived(prefs.resolvedTheme === 'dark' ? 'light' : 'dark');

  async function signOut() {
    try {
      await logout(false);
      moreOpen = false;
      onlogout();
    } catch (reason) {
      toasts.error('Sign out failed', { description: errorMessage(reason) });
    }
  }
</script>

<nav class="tabbar" aria-label="Primary navigation">
  <ul>
    {#each primary as item (item.route)}
      {@const active = item.matches.includes(router.name ?? 'overview')}
      {@const badge = item.route === 'alerts' ? shell.openIncidents : null}
      <li>
        <a
          href={item.href}
          class:active
          aria-current={active ? 'page' : undefined}
        >
          <span class="icon">
            <item.icon aria-hidden="true" />
            {#if badge}<span class="badge" aria-hidden="true">{badge}</span
              >{/if}
          </span>
          <span class="label">{item.label}</span>
          {#if badge}<span class="sr-only">, {badge} open incidents</span>{/if}
        </a>
      </li>
    {/each}
    <li>
      <button
        type="button"
        class:active={moreActive}
        aria-haspopup="dialog"
        aria-expanded={moreOpen}
        onclick={() => (moreOpen = true)}
      >
        <span class="icon"><Ellipsis aria-hidden="true" /></span>
        <span class="label">More</span>
      </button>
    </li>
  </ul>
</nav>

<Dialog bind:open={moreOpen} title="More" size="sm">
  <ul class="more-list">
    {#each secondary as item (item.route)}
      <li>
        <a href={item.href} onclick={() => (moreOpen = false)}>
          <item.icon aria-hidden="true" />
          {item.label}
        </a>
      </li>
    {/each}
    <li>
      <button
        type="button"
        onclick={() => {
          void prefs.save({ theme: nextTheme }).catch((reason) =>
            toasts.error('Theme could not be saved', {
              description: errorMessage(reason),
            }),
          );
          moreOpen = false;
        }}
      >
        {#if nextTheme === 'dark'}<Moon aria-hidden="true" />{:else}<Sun
            aria-hidden="true"
          />{/if}
        Switch to {nextTheme} theme
      </button>
    </li>
    <li>
      <button type="button" onclick={signOut}>
        <LogOut aria-hidden="true" />
        Sign out {session.user.username}
      </button>
    </li>
  </ul>
</Dialog>

<style>
  .tabbar {
    position: fixed;
    inset: auto 0 0 0;
    z-index: 40;
    height: calc(var(--tabbar-h) + env(safe-area-inset-bottom));
    padding-bottom: env(safe-area-inset-bottom);
    border-top: 1px solid var(--border);
    background: var(--surface);
  }
  ul {
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    height: var(--tabbar-h);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  a,
  button {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 3px;
    width: 100%;
    height: 100%;
    padding: 0;
    border: 0;
    background: none;
    color: var(--text-3);
    font-size: 11px;
    font-weight: 500;
    text-decoration: none;
  }
  a.active,
  button.active {
    color: var(--accent-text);
  }
  .icon {
    position: relative;
    display: inline-flex;
  }
  .icon :global(svg) {
    width: 22px;
    height: 22px;
  }
  .badge {
    position: absolute;
    top: -5px;
    right: -9px;
    min-width: 16px;
    height: 16px;
    padding: 0 4px;
    border-radius: var(--radius-full);
    background: var(--critical-solid);
    color: #fff;
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 700;
    line-height: 16px;
    text-align: center;
  }
  .more-list {
    display: grid;
    gap: 2px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .more-list a,
  .more-list button {
    flex-direction: row;
    justify-content: flex-start;
    gap: var(--space-3);
    height: 44px;
    padding: 0 var(--space-3);
    border-radius: var(--radius-sm);
    color: var(--text);
    font-size: var(--text-base);
  }
  .more-list a:hover,
  .more-list button:hover {
    background: var(--surface-2);
  }
  .more-list :global(svg) {
    width: 18px;
    height: 18px;
    color: var(--text-2);
  }
</style>
