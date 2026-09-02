<script lang="ts">
  import ChevronsUpDown from '@lucide/svelte/icons/chevrons-up-down';
  import LogOut from '@lucide/svelte/icons/log-out';
  import Moon from '@lucide/svelte/icons/moon';
  import Sun from '@lucide/svelte/icons/sun';
  import MonitorCog from '@lucide/svelte/icons/monitor-cog';
  import Settings from '@lucide/svelte/icons/settings';
  import Rows3 from '@lucide/svelte/icons/rows-3';
  import type { SessionInfo } from '../auth';
  import { logout } from '../auth';
  import { prefs } from '../preferences.svelte';
  import { errorMessage } from '../api/client';
  import Menu from '../ui/Menu.svelte';
  import MenuItem from '../ui/MenuItem.svelte';
  import MenuSeparator from '../ui/MenuSeparator.svelte';
  import ConfirmDialog from '../ui/ConfirmDialog.svelte';
  import { toasts } from '../ui/toast.svelte';
  import { formatAbsolute } from '../ui/relative-time';

  let {
    session,
    onlogout,
    collapsed = false,
  }: {
    session: SessionInfo;
    onlogout: () => void;
    collapsed?: boolean;
  } = $props();

  let confirmAll = $state(false);
  const initial = $derived(session.user.username.slice(0, 1).toUpperCase());
  const nextTheme = $derived(prefs.resolvedTheme === 'dark' ? 'light' : 'dark');

  async function signOut(all: boolean) {
    try {
      await logout(all);
      onlogout();
    } catch (reason) {
      toasts.error('Sign out failed', { description: errorMessage(reason) });
      throw reason;
    }
  }

  async function setTheme(theme: 'light' | 'dark' | 'system') {
    try {
      await prefs.save({ theme });
    } catch (reason) {
      toasts.error('Theme could not be saved', {
        description: errorMessage(reason),
      });
    }
  }

  async function toggleDensity() {
    try {
      await prefs.save({
        density: prefs.value.density === 'compact' ? 'comfortable' : 'compact',
      });
    } catch (reason) {
      toasts.error('Density could not be saved', {
        description: errorMessage(reason),
      });
    }
  }
</script>

<Menu label="Account" placement={collapsed ? 'right-end' : 'top-start'}>
  {#snippet trigger(props)}
    <button
      type="button"
      class="trigger"
      class:collapsed
      {...props}
      aria-label={collapsed ? `Account: ${session.user.username}` : undefined}
    >
      <span class="avatar" aria-hidden="true">{initial}</span>
      {#if !collapsed}
        <span class="who">
          <span class="name">{session.user.username}</span>
          <span class="role">Administrator</span>
        </span>
        <ChevronsUpDown class="chevron" aria-hidden="true" />
      {/if}
    </button>
  {/snippet}
  <div class="session-meta">
    <strong>{session.user.username}</strong>
    <span>Session ends {formatAbsolute(session.expiresAt)}</span>
  </div>
  <MenuSeparator />
  <MenuItem onselect={() => void setTheme(nextTheme)}>
    {#snippet icon()}{#if nextTheme === 'dark'}<Moon />{:else}<Sun
        />{/if}{/snippet}
    Switch to {nextTheme} theme
  </MenuItem>
  {#if prefs.value.theme !== 'system'}
    <MenuItem onselect={() => void setTheme('system')}>
      {#snippet icon()}<MonitorCog />{/snippet}
      Follow system theme
    </MenuItem>
  {/if}
  <MenuItem onselect={() => void toggleDensity()}>
    {#snippet icon()}<Rows3 />{/snippet}
    {prefs.value.density === 'compact'
      ? 'Comfortable density'
      : 'Compact density'}
  </MenuItem>
  <MenuItem href="/settings/appearance">
    {#snippet icon()}<Settings />{/snippet}
    Appearance settings
  </MenuItem>
  <MenuSeparator />
  <MenuItem onselect={() => void signOut(false)}>
    {#snippet icon()}<LogOut />{/snippet}
    Sign out
  </MenuItem>
  <MenuItem danger onselect={() => (confirmAll = true)}>
    {#snippet icon()}<LogOut />{/snippet}
    Sign out everywhere…
  </MenuItem>
</Menu>

<ConfirmDialog
  bind:open={confirmAll}
  title="Sign out everywhere?"
  description="Every active session for this administrator will end, including this one."
  confirmLabel="Sign out everywhere"
  onconfirm={() => signOut(true)}
/>

<style>
  .trigger {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    padding: var(--space-2);
    border: 1px solid transparent;
    border-radius: var(--radius);
    background: none;
    color: var(--text);
    text-align: left;
  }
  .trigger:hover,
  .trigger[aria-expanded='true'] {
    background: var(--surface-2);
    border-color: var(--border);
  }
  .trigger.collapsed {
    justify-content: center;
    width: auto;
    padding: var(--space-1);
  }
  .avatar {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: 30px;
    height: 30px;
    border-radius: var(--radius-sm);
    background: var(--accent-bg);
    color: var(--accent-text);
    font-size: var(--text-sm);
    font-weight: 700;
  }
  .who {
    display: grid;
    min-width: 0;
    flex: 1;
    line-height: 1.2;
  }
  .name {
    font-size: var(--text-sm);
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .role {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .trigger :global(.chevron) {
    width: 14px;
    height: 14px;
    color: var(--text-3);
  }
  .session-meta {
    display: grid;
    gap: 2px;
    padding: var(--space-2) var(--space-2) var(--space-2);
    font-size: var(--text-sm);
  }
  .session-meta span {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
</style>
