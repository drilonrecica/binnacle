<script lang="ts">
  import PanelLeftClose from '@lucide/svelte/icons/panel-left-close';
  import PanelLeftOpen from '@lucide/svelte/icons/panel-left-open';
  import type { SessionInfo } from '../auth';
  import type { LiveStore } from '../live.svelte';
  import { router } from '../router.svelte';
  import { prefs } from '../preferences.svelte';
  import { tooltip } from '../ui/tooltip';
  import { navigation } from './navigation';
  import { shell } from './shell-state.svelte';
  import ConnectionStatus from './ConnectionStatus.svelte';
  import UserMenu from './UserMenu.svelte';

  let {
    session,
    live,
    onlogout,
  }: {
    session: SessionInfo;
    live: LiveStore;
    onlogout: () => void;
  } = $props();

  const collapsed = $derived(prefs.sidebarCollapsed);
  const degraded = $derived(
    live.snapshot?.resources.filter(
      (resource) =>
        resource.status === 'degraded' || resource.status === 'down',
    ).length ?? 0,
  );

  function badgeFor(route: string): number | null {
    if (route === 'alerts')
      return shell.openIncidents && shell.openIncidents > 0
        ? shell.openIncidents
        : null;
    if (route === 'resources') return degraded > 0 ? degraded : null;
    return null;
  }
</script>

<aside class="sidebar" class:collapsed>
  <a class="brand" href="/overview" aria-label="Binnacle overview">
    <img
      class="mark dark"
      src="/brand/binnacle-mark-dark.png"
      alt=""
      width="28"
      height="28"
    />
    <img
      class="mark light"
      src="/brand/binnacle-mark.png"
      alt=""
      width="28"
      height="28"
    />
    {#if !collapsed}<span class="wordmark">Binnacle</span>{/if}
  </a>

  <nav class="nav" aria-label="Primary navigation">
    <ul>
      {#each navigation as item (item.route)}
        {@const active = item.matches.includes(router.name ?? 'overview')}
        {@const badge = badgeFor(item.route)}
        <li>
          <a
            href={item.href}
            class="nav-link"
            class:active
            aria-current={active ? 'page' : undefined}
            use:tooltip={collapsed ? item.label : null}
          >
            <span class="icon"><item.icon aria-hidden="true" /></span>
            {#if !collapsed}<span class="label">{item.label}</span>{/if}
            {#if badge}
              <span class="badge" class:dot={collapsed} data-route={item.route}>
                {#if !collapsed}{badge}{/if}
                <span class="sr-only"
                  >{badge}
                  {item.route === 'alerts'
                    ? 'open incidents'
                    : 'resources need attention'}</span
                >
              </span>
            {/if}
          </a>
        </li>
      {/each}
    </ul>
  </nav>

  <div class="bottom">
    <div class="status-row">
      <ConnectionStatus {live} compact={collapsed} />
    </div>
    <UserMenu {session} {onlogout} {collapsed} />
    <button
      type="button"
      class="collapse"
      onclick={() => prefs.toggleSidebar()}
      aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
      aria-expanded={!collapsed}
      use:tooltip={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
    >
      {#if collapsed}<PanelLeftOpen aria-hidden="true" />{:else}<PanelLeftClose
          aria-hidden="true"
        />{/if}
    </button>
  </div>
</aside>

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    width: var(--sidebar-w);
    height: 100vh;
    height: 100dvh;
    padding: var(--space-3);
    border-right: 1px solid var(--border);
    background: var(--surface);
    transition: width var(--motion) var(--ease);
  }
  .sidebar.collapsed {
    width: var(--sidebar-rail-w);
    padding: var(--space-3) var(--space-2);
    align-items: center;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    height: 40px;
    margin-bottom: var(--space-4);
    padding: 0 var(--space-2);
    border-radius: var(--radius-sm);
    color: var(--text);
    text-decoration: none;
  }
  .brand:hover {
    text-decoration: none;
  }
  .collapsed .brand {
    justify-content: center;
    padding: 0;
    width: 40px;
  }
  .mark {
    width: 28px;
    height: 28px;
    flex: none;
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
  .wordmark {
    font-size: var(--text-md);
    font-weight: 700;
    letter-spacing: -0.02em;
  }
  .nav ul {
    display: grid;
    gap: 2px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .nav-link {
    position: relative;
    display: flex;
    align-items: center;
    gap: var(--space-3);
    height: 36px;
    padding: 0 var(--space-2);
    border-radius: var(--radius-sm);
    color: var(--text-2);
    font-size: var(--text-sm);
    font-weight: 500;
    text-decoration: none;
    transition:
      background var(--motion-fast) var(--ease),
      color var(--motion-fast) var(--ease);
  }
  .collapsed .nav-link {
    justify-content: center;
    width: 40px;
    padding: 0;
  }
  .nav-link:hover {
    background: var(--surface-2);
    color: var(--text);
    text-decoration: none;
  }
  .nav-link.active {
    background: var(--accent-bg);
    color: var(--accent-text);
  }
  .nav-link.active::before {
    content: '';
    position: absolute;
    left: -12px;
    top: 8px;
    bottom: 8px;
    width: 3px;
    border-radius: 0 3px 3px 0;
    background: var(--accent);
  }
  .collapsed .nav-link.active::before {
    left: -8px;
  }
  .icon {
    display: inline-flex;
    flex: none;
  }
  .icon :global(svg) {
    width: 18px;
    height: 18px;
  }
  .label {
    flex: 1;
    white-space: nowrap;
  }
  .badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 20px;
    height: 20px;
    padding: 0 6px;
    border-radius: var(--radius-full);
    background: var(--warn-bg);
    color: var(--warn-fg);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    font-weight: 600;
  }
  .badge[data-route='alerts'] {
    background: var(--critical-bg);
    color: var(--critical-fg);
  }
  .badge.dot {
    position: absolute;
    top: 6px;
    right: 6px;
    min-width: 8px;
    width: 8px;
    height: 8px;
    padding: 0;
    background: var(--warn-solid);
  }
  .badge.dot[data-route='alerts'] {
    background: var(--critical-solid);
  }
  .bottom {
    display: grid;
    gap: var(--space-2);
    margin-top: auto;
    padding-top: var(--space-3);
    border-top: 1px solid var(--border);
  }
  .collapsed .bottom {
    justify-items: center;
  }
  .status-row {
    display: flex;
    justify-content: flex-start;
    padding: 0 var(--space-1);
  }
  .collapsed .status-row {
    justify-content: center;
    padding: 0;
  }
  .collapse {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    height: 32px;
    padding: 0 var(--space-2);
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--text-3);
  }
  .collapse:hover {
    background: var(--surface-2);
    color: var(--text);
  }
  .collapse :global(svg) {
    width: 16px;
    height: 16px;
  }
</style>
