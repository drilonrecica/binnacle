<script lang="ts">
  import type { LiveStore } from '../live.svelte';
  import { homePath } from '../router';
  import { router } from '../router.svelte';
  import Overview from '../pages/Overview.svelte';
  import Host from '../pages/Host.svelte';
  import Resources from '../pages/Resources.svelte';
  import Activity from '../pages/Activity.svelte';
  import Alerts from '../pages/Alerts.svelte';
  import Incident from '../pages/Incident.svelte';
  import ResourceDetail from '../pages/ResourceDetail.svelte';
  import Settings from '../pages/Settings.svelte';
  import Button from '../ui/Button.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import PageHeader from '../ui/PageHeader.svelte';

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

{#if router.name === 'headroom'}
  <Overview {live} />
{:else if router.name === 'resource'}
  <!-- Keyed by id: moving between two resources rebuilds the page, so
       no event, chart or request of the previous one survives (UI-4). -->
  {#key router.params.id}
    <ResourceDetail {live} id={router.params.id} />
  {/key}
{:else if router.name === 'resources'}
  <Resources {live} />
{:else if router.name === 'server'}
  <Host {live} />
{:else if router.name === 'changes'}
  <Activity {live} />
{:else if router.name === 'incident'}
  {#key router.params.id}
    <Incident {live} id={router.params.id} {viewer} />
  {/key}
{:else if router.name === 'incidents'}
  <Alerts {live} />
{:else if router.name === 'settings'}
  <Settings {live} section={router.params.section} {onsignedout} />
{:else}
  <PageHeader
    title="Page not found"
    description="This address does not match a page in Binnacle."
  />
  <EmptyState title="Nothing here">
    <Button href={homePath} variant="primary">Go to Headroom</Button>
  </EmptyState>
{/if}
