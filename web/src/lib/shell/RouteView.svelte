<script lang="ts">
  import type { LiveStore } from '../live.svelte';
  import { homePath } from '../router';
  import { router } from '../router.svelte';
  import Overview from '../pages/Overview.svelte';
  import Resources from '../pages/Resources.svelte';
  import ResourceDetail from '../pages/ResourceDetail.svelte';
  import Button from '../ui/Button.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import ErrorState from '../ui/ErrorState.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import Skeleton from '../ui/Skeleton.svelte';

  /** The page for the current route inside the authenticated shell. */
  let {
    live,
    viewer = '',
    onsignedout,
  }: {
    live: LiveStore;
    /** The signed-in username, for the local "seen by" marker. */
    viewer?: string;
    onsignedout: () => void;
  } = $props();
</script>

<!-- Settings, Server, Incidents and Changes load on first visit (UI-16), so
     the sign-in screen and Headroom do not download them. -->
{#snippet pending()}
  <div role="status" aria-busy="true">
    <span class="sr-only">Loading page…</span>
    <Skeleton height={28} width="40%" />
    <Skeleton height={160} lines={2} />
  </div>
{/snippet}
{#snippet failed()}
  <ErrorState
    title="This page could not be loaded"
    description="Check your connection, then reload to try again."
    onretry={() => location.reload()}
    retryLabel="Reload"
  />
{/snippet}

{#if router.name === 'headroom'}
  <Overview {live} />
{:else if router.name === 'resource'}
  <!-- Keyed by id: moving between two resources rebuilds the page, so
       no event, chart or request of the previous one survives (UI-4). -->
  {#key router.params.id}
    <ResourceDetail {live} id={router.params.id ?? ''} />
  {/key}
{:else if router.name === 'resources'}
  <Resources {live} />
{:else if router.name === 'server'}
  {#await import('../pages/Host.svelte')}
    {@render pending()}
  {:then { default: Host }}
    <Host {live} />
  {:catch}
    {@render failed()}
  {/await}
{:else if router.name === 'changes'}
  {#await import('../pages/Activity.svelte')}
    {@render pending()}
  {:then { default: Activity }}
    <Activity {live} />
  {:catch}
    {@render failed()}
  {/await}
{:else if router.name === 'incident'}
  {#key router.params.id}
    {#await import('../pages/Incident.svelte')}
      {@render pending()}
    {:then { default: Incident }}
      <Incident {live} id={router.params.id ?? ''} {viewer} />
    {:catch}
      {@render failed()}
    {/await}
  {/key}
{:else if router.name === 'incidents'}
  {#await import('../pages/Alerts.svelte')}
    {@render pending()}
  {:then { default: Alerts }}
    <Alerts {live} />
  {:catch}
    {@render failed()}
  {/await}
{:else if router.name === 'settings'}
  {#await import('../pages/Settings.svelte')}
    {@render pending()}
  {:then { default: Settings }}
    <Settings
      {live}
      section={router.params.section ?? 'general'}
      {onsignedout}
    />
  {:catch}
    {@render failed()}
  {/await}
{:else}
  <PageHeader
    title="Page not found"
    description="This address does not match a page in Binnacle."
  />
  <EmptyState title="Nothing here">
    <Button href={homePath} variant="primary">Go to Headroom</Button>
  </EmptyState>
{/if}
