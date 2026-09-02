<script lang="ts">
  import { onMount } from 'svelte';
  import BellRing from '@lucide/svelte/icons/bell-ring';
  import Database from '@lucide/svelte/icons/database';
  import HeartPulse from '@lucide/svelte/icons/heart-pulse';
  import KeyRound from '@lucide/svelte/icons/key-round';
  import Palette from '@lucide/svelte/icons/palette';
  import Plug from '@lucide/svelte/icons/plug';
  import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal';
  import type { LiveStore } from '../live.svelte';
  import {
    loadSettings,
    saveSettings,
    type SettingsSnapshot,
  } from '../api/settings';
  import { errorMessage } from '../api/client';
  import { settingsSections, type SettingsSection } from '../router';
  import { viewport } from '../ui/media.svelte';
  import AccessSettings from '../settings/AccessSettings.svelte';
  import AppearanceSettings from '../settings/AppearanceSettings.svelte';
  import DataSettings from '../settings/DataSettings.svelte';
  import GeneralSettings from '../settings/GeneralSettings.svelte';
  import IntegrationsSettings from '../settings/IntegrationsSettings.svelte';
  import NotificationsSettings from '../settings/NotificationsSettings.svelte';
  import SystemSettings from '../settings/SystemSettings.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import Skeleton from '../ui/Skeleton.svelte';

  let {
    live,
    section,
    onsignedout,
  }: {
    live: LiveStore;
    section: string;
    onsignedout: () => void;
  } = $props();

  let snapshot = $state<SettingsSnapshot | null>(null);
  let error = $state('');

  const active = $derived<SettingsSection>(
    (settingsSections as readonly string[]).includes(section)
      ? (section as SettingsSection)
      : 'general',
  );

  const items: Array<{
    id: SettingsSection;
    label: string;
    description: string;
    icon: typeof Database;
  }> = [
    {
      id: 'general',
      label: 'General',
      description: 'Collection and charts',
      icon: SlidersHorizontal,
    },
    {
      id: 'data',
      label: 'Data & retention',
      description: 'History, storage budget, deletion',
      icon: Database,
    },
    {
      id: 'access',
      label: 'Access',
      description: 'Sessions, two-factor, sign-in',
      icon: KeyRound,
    },
    {
      id: 'notifications',
      label: 'Notifications',
      description: 'Channels and delivery history',
      icon: BellRing,
    },
    {
      id: 'appearance',
      label: 'Appearance',
      description: 'Theme, density, pins',
      icon: Palette,
    },
    {
      id: 'integrations',
      label: 'Integrations',
      description: 'Coolify, tokens, Prometheus',
      icon: Plug,
    },
    {
      id: 'system',
      label: 'System',
      description: 'Health, diagnostics, deployment',
      icon: HeartPulse,
    },
  ];

  const current = $derived(
    items.find((item) => item.id === active) ?? items[0],
  );

  async function apply(changes: Record<string, string>) {
    if (!snapshot) throw new Error('Settings have not loaded yet.');
    snapshot = await saveSettings(snapshot.revision, changes);
    return snapshot;
  }

  onMount(() => {
    void loadSettings()
      .then((value) => (snapshot = value))
      .catch((reason) => (error = errorMessage(reason)));
  });
</script>

<PageHeader
  title="Settings"
  description="Everything Binnacle can be told from the browser. Deployment-level options are shown read-only under System."
/>

<div class="layout" class:mobile={viewport.isMobile}>
  <nav class="subnav" aria-label="Settings sections">
    <ul>
      {#each items as item (item.id)}
        <li>
          <a
            href={`/settings/${item.id}`}
            class:active={item.id === active}
            aria-current={item.id === active ? 'page' : undefined}
          >
            <span class="icon"><item.icon aria-hidden="true" /></span>
            <span class="text">
              <span class="label">{item.label}</span>
              {#if !viewport.isMobile}<span class="description"
                  >{item.description}</span
                >{/if}
            </span>
          </a>
        </li>
      {/each}
    </ul>
  </nav>

  <section class="content" aria-labelledby="settings-section-title">
    <h2 id="settings-section-title" class="sr-only">{current.label}</h2>
    {#if error}
      <p class="error" role="alert">{error}</p>
    {:else if !snapshot && active !== 'appearance' && active !== 'notifications'}
      <div class="loading" role="status" aria-label="Loading settings">
        <Skeleton height={180} /><Skeleton height={180} />
      </div>
    {:else if active === 'general'}
      <GeneralSettings {snapshot} {apply} />
    {:else if active === 'data'}
      <DataSettings {snapshot} {apply} live={live.snapshot} />
    {:else if active === 'access'}
      <AccessSettings {snapshot} {apply} {onsignedout} />
    {:else if active === 'notifications'}
      <NotificationsSettings />
    {:else if active === 'appearance'}
      <AppearanceSettings live={live.snapshot} />
    {:else if active === 'integrations'}
      <IntegrationsSettings {snapshot} />
    {:else if active === 'system'}
      <SystemSettings {snapshot} />
    {/if}
  </section>
</div>

<style>
  .layout {
    display: grid;
    grid-template-columns: 240px minmax(0, 1fr);
    gap: var(--space-6);
    align-items: start;
  }
  .layout.mobile {
    grid-template-columns: 1fr;
    gap: var(--space-4);
  }
  .subnav {
    position: sticky;
    top: calc(var(--topbar-h) + var(--space-4));
  }
  .mobile .subnav {
    position: static;
    overflow-x: auto;
  }
  .subnav ul {
    display: grid;
    gap: 2px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .mobile .subnav ul {
    display: flex;
    gap: var(--space-1);
  }
  .subnav a {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-sm);
    color: var(--text-2);
    text-decoration: none;
  }
  .mobile .subnav a {
    flex: none;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border);
    background: var(--surface);
  }
  .subnav a:hover {
    background: var(--surface-2);
    color: var(--text);
  }
  .subnav a.active {
    background: var(--accent-bg);
    color: var(--accent-text);
  }
  .icon {
    display: inline-flex;
    flex: none;
  }
  .icon :global(svg) {
    width: 16px;
    height: 16px;
  }
  .text {
    display: grid;
    line-height: 1.25;
  }
  .label {
    font-size: var(--text-sm);
    font-weight: 500;
  }
  .description {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .subnav a.active .description {
    color: var(--accent-text);
    opacity: 0.8;
  }
  .content {
    min-width: 0;
  }
  .loading {
    display: grid;
    gap: var(--space-4);
  }
  .error {
    color: var(--critical-fg);
  }
</style>
