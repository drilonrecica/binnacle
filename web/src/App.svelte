<script lang="ts">
  import { onMount } from 'svelte';
  import { LiveStore } from './lib/live.svelte';
  import { liveSeries } from './lib/live-series.svelte';
  import { fetchMetrics } from './lib/api/metrics';
  import { prefs } from './lib/preferences.svelte';
  import { interceptLinks, router } from './lib/router.svelte';
  import { routeDefinition } from './lib/router';
  import { pageTitle } from './lib/shell/navigation';
  import { shell } from './lib/shell/shell-state.svelte';
  import AppShell from './lib/shell/AppShell.svelte';
  import {
    authMethods,
    bootstrapExternalSession,
    currentSession,
    type SessionInfo,
  } from './lib/auth';
  import { onboardingState, setupAvailable } from './lib/onboarding';
  import Overview from './lib/pages/Overview.svelte';
  import Host from './lib/pages/Host.svelte';
  import Resources from './lib/pages/Resources.svelte';
  import Activity from './lib/pages/Activity.svelte';
  import Logs from './lib/pages/Logs.svelte';
  import Alerts from './lib/pages/Alerts.svelte';
  import Incident from './lib/pages/Incident.svelte';
  import ResourceDetail from './lib/pages/ResourceDetail.svelte';
  import Settings from './lib/pages/Settings.svelte';
  import Login from './lib/pages/Login.svelte';
  import Setup from './lib/pages/Setup.svelte';
  import Onboarding from './lib/pages/Onboarding.svelte';

  const live = new LiveStore();

  let loading = $state(true);
  let session = $state<SessionInfo | null>(null);

  const allowed = $derived(session !== null);
  const definition = $derived(
    router.match ? routeDefinition(router.match.name) : null,
  );

  $effect(() => {
    if (live.snapshot) {
      liveSeries.push(live.snapshot);
      void liveSeries.ensureHostSeeded(
        (metrics, from, to) => fetchMetrics('host', metrics, from, to),
        live.snapshot.host.memoryTotalBytes ?? null,
      );
    }
  });

  function startSession(value: SessionInfo) {
    session = value;
    live.connect();
    shell.startIncidentPolling();
  }

  function endSession() {
    live.close();
    shell.stopIncidentPolling();
    session = null;
  }

  onMount(() => {
    const startedAtRoot = location.pathname === '/';
    prefs.bootstrap();
    router.install();
    const stopLinks = interceptLinks();

    void currentSession()
      .catch(() => null)
      .then(async (value) => {
        if (value) return value;
        try {
          const methods = await authMethods();
          if (methods.proxyAvailable) {
            await bootstrapExternalSession();
            return await currentSession();
          }
        } catch {
          /* normal local-login path */
        }
        return null;
      })
      .then((value) => {
        loading = false;
        if (value) {
          startSession(value);
          if (!router.match || definition?.public) {
            router.navigate('/overview', { replace: true });
          }
          void prefs
            .load()
            .then((saved) => {
              if (startedAtRoot)
                router.navigate(`/${saved.landingPage}`, { replace: true });
            })
            .catch(() => {
              /* The local mirror remains a usable fallback. */
            });
          void onboardingState()
            .then((onboarding) => {
              if (!onboarding.completedAt && router.name !== 'onboarding') {
                router.navigate('/onboarding', { replace: true });
              }
            })
            .catch(() => {
              /* Onboarding state is advisory; the dashboard still works. */
            });
        } else if (!definition?.public) {
          void setupAvailable().then((available) => {
            router.navigate(available ? '/setup' : '/login', { replace: true });
          });
        }
      });

    return () => {
      stopLinks();
      endSession();
    };
  });

  function authenticated(path: string) {
    void currentSession().then((value) => {
      if (!value) return;
      startSession(value);
      router.navigate(path, { replace: true });
    });
  }

  function signedOut() {
    endSession();
    router.navigate('/login', { replace: true });
  }

  function setupClaimed() {
    authenticated('/onboarding');
  }

  function onboardingComplete() {
    router.navigate('/overview', { replace: true });
  }

  const title = $derived(pageTitle(router.name));
</script>

<svelte:head><title>Binnacle — {title}</title></svelte:head>

<a class="skip-link" href="#content">Skip to content</a>
{#if loading}
  <main class="access-state" aria-busy="true">
    <img src="/brand/binnacle-mark-dark.png" alt="" />
    <p>Checking access…</p>
  </main>
{:else if !allowed}
  <main id="content" class="public-shell">
    {#if router.name === 'setup'}<Setup onclaimed={setupClaimed} />
    {:else}<Login onauthenticated={authenticated} />{/if}
  </main>
{:else if router.name === 'onboarding'}
  <main id="content" class="public-shell onboarding-shell">
    <Onboarding oncomplete={onboardingComplete} />
  </main>
{:else if session}
  <AppShell {session} {live} onlogout={signedOut}>
    {#if router.name === 'overview'}
      <Overview {live} />
    {:else if router.name === 'resource'}
      <ResourceDetail {live} id={router.params.id} />
    {:else if router.name === 'resources'}
      <Resources {live} />
    {:else if router.name === 'host'}
      <Host {live} />
    {:else if router.name === 'activity'}
      <Activity {live} />
    {:else if router.name === 'logs'}
      <Logs {live} />
    {:else if router.name === 'incident'}
      <Incident {live} id={router.params.id} />
    {:else if router.name === 'alerts'}
      <Alerts {live} />
    {:else if router.name === 'settings'}
      <Settings
        {live}
        section={router.params.section}
        onsignedout={signedOut}
      />
    {/if}
  </AppShell>
{/if}
