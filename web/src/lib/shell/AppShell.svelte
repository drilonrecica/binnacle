<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { SessionInfo } from '../auth';
  import type { LiveStore } from '../live.svelte';
  import { prefs } from '../preferences.svelte';
  import { resourcePath } from '../router';
  import { viewport } from '../ui/media.svelte';
  import CommandPalette from '../ui/CommandPalette.svelte';
  import type { PaletteItem } from '../ui/palette';
  import Toaster from '../ui/Toaster.svelte';
  import { errorMessage } from '../api/client';
  import { toasts } from '../ui/toast.svelte';
  import ConnectionBar from './ConnectionBar.svelte';
  import MobileNav from './MobileNav.svelte';
  import Sidebar from './Sidebar.svelte';
  import TopBar from './TopBar.svelte';
  import { navigation } from './navigation';
  import { shell } from './shell-state.svelte';

  let {
    session,
    live,
    onlogout,
    children,
  }: {
    session: SessionInfo;
    live: LiveStore;
    onlogout: () => void;
    children: Snippet;
  } = $props();

  function paletteItems(): PaletteItem[] {
    const pages: PaletteItem[] = navigation.map((item) => ({
      id: `page:${item.route}`,
      group: 'Pages',
      label: item.label,
      href: item.href,
      keywords: item.keywords,
    }));
    const settings: PaletteItem[] = [
      ['general', 'General settings'],
      ['data', 'Data & retention'],
      ['access', 'Access & sessions'],
      ['notifications', 'Notification channels'],
      ['appearance', 'Appearance'],
      ['integrations', 'Integrations & API'],
      ['system', 'System health & diagnostics'],
    ].map(([section, label]) => ({
      id: `settings:${section}`,
      group: 'Settings',
      label,
      href: `/settings/${section}`,
      keywords: ['settings'],
    }));
    const resources: PaletteItem[] = (live.snapshot?.resources ?? []).map(
      (resource) => ({
        id: `resource:${resource.id}`,
        group: 'Resources',
        label: resource.name,
        hint:
          [resource.project, resource.environment]
            .filter(Boolean)
            .join(' / ') ||
          resource.context ||
          resource.category ||
          '',
        href: resourcePath(resource.id),
        keywords: [resource.id, resource.status],
      }),
    );
    const actions: PaletteItem[] = [
      {
        id: 'action:theme',
        group: 'Actions',
        label: `Switch to ${prefs.resolvedTheme === 'dark' ? 'light' : 'dark'} theme`,
        keywords: ['theme', 'dark', 'light', 'appearance'],
        action: () =>
          void prefs
            .save({ theme: prefs.resolvedTheme === 'dark' ? 'light' : 'dark' })
            .catch((reason) =>
              toasts.error('Theme could not be saved', {
                description: errorMessage(reason),
              }),
            ),
      },
      {
        id: 'action:density',
        group: 'Actions',
        label:
          prefs.value.density === 'compact'
            ? 'Use comfortable density'
            : 'Use compact density',
        keywords: ['density', 'compact', 'comfortable'],
        action: () =>
          void prefs
            .save({
              density:
                prefs.value.density === 'compact' ? 'comfortable' : 'compact',
            })
            .catch((reason) =>
              toasts.error('Density could not be saved', {
                description: errorMessage(reason),
              }),
            ),
      },
      {
        id: 'action:silence',
        group: 'Actions',
        label: 'Create a silence…',
        href: '/alerts?tab=silences&new=1',
        keywords: ['mute', 'quiet', 'maintenance'],
      },
    ];
    return [...pages, ...resources, ...actions, ...settings];
  }
</script>

<div
  class="shell"
  class:collapsed={prefs.sidebarCollapsed}
  class:mobile={viewport.isMobile}
>
  {#if !viewport.isMobile}
    <Sidebar {session} {live} {onlogout} />
  {/if}
  <div class="column">
    <TopBar {live} />
    <ConnectionBar {live} />
    <main id="content" class="content" tabindex="-1">
      {@render children()}
    </main>
  </div>
  {#if viewport.isMobile}
    <MobileNav {session} {onlogout} />
  {/if}
</div>

<CommandPalette bind:open={shell.paletteOpen} items={paletteItems} />
<Toaster />

<style>
  .shell {
    display: flex;
    min-height: 100vh;
    min-height: 100dvh;
  }
  .shell > :global(.sidebar) {
    position: sticky;
    top: 0;
    flex: none;
  }
  .column {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
  }
  .column > :global(.topbar) {
    position: sticky;
    top: 0;
    z-index: 35;
  }
  .content {
    flex: 1;
    width: 100%;
    max-width: var(--content-max);
    margin: 0 auto;
    padding: var(--space-5);
    outline: none;
  }
  .mobile .content {
    padding: var(--space-4) var(--space-4)
      calc(var(--tabbar-h) + var(--space-6));
  }
</style>
