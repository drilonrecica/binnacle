<script lang="ts">
  import { onMount } from 'svelte';
  import { LiveStore, sessionActive } from './lib/live';

  const live = new LiveStore();
  const routes = [
    'overview',
    'resources',
    'server',
    'events',
    'checks',
    'settings',
  ];
  let loading = $state(true);
  let allowed = $state(false);
  let route = $state(location.pathname.split('/')[1] || 'overview');
  onMount(() => {
    void sessionActive()
      .catch(() => false)
      .then((active) => {
        allowed = active;
        loading = false;
        if (allowed) live.connect();
        else if (route !== 'login' && route !== 'setup') {
          history.pushState({}, '', '/login');
          route = 'login';
        }
      });
    return () => live.close();
  });
</script>

<svelte:head><title>TALOS</title></svelte:head>
<a class="skip" href="#content">Skip to content</a>
{#if loading}
  <main aria-busy="true"><p>Checking access…</p></main>
{:else if !allowed}
  <main id="content">
    <h1>{route === 'setup' ? 'Setup TALOS' : 'Sign in to TALOS'}</h1>
    <p>Authentication is not configured in this build.</p>
  </main>
{:else}
  <div class="shell">
    <header>
      <a
        href="/overview"
        onclick={(e) => {
          e.preventDefault();
          history.pushState({}, '', '/overview');
          route = 'overview';
        }}>TALOS</a
      ><span>Live monitoring</span>
    </header>
    <nav aria-label="Primary navigation">
      {#each routes as item (item)}<a
          href="/{item}"
          aria-current={route === item ? 'page' : undefined}
          onclick={(e) => {
            e.preventDefault();
            history.pushState({}, '', `/${item}`);
            route = item;
          }}>{item}</a
        >{/each}
    </nav>
    <main id="content">
      <h1>{route[0].toUpperCase() + route.slice(1)}</h1>
      {#if route === 'checks'}<p>
          Checks are planned for a later release.
        </p>{:else}<p>
          {live.state === 'connected'
            ? 'Live connection active.'
            : 'Connecting to live monitoring…'}
        </p>{/if}
    </main>
  </div>
{/if}
