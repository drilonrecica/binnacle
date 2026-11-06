<script lang="ts">
  import { onMount } from 'svelte';
  import { LiveStore, sessionActive } from './lib/live.svelte';
  import { liveSeries } from './lib/live-series.svelte';
  import { fetchMetrics, fetchSparklines } from './lib/api/metrics';
  import { prefs } from './lib/preferences.svelte';
  import { interceptLinks, router } from './lib/router.svelte';
  import {
    homePath,
    landingPath,
    loginPath,
    routeDefinition,
  } from './lib/router';
  import { sessionEndedEvent } from './lib/api/client';
  import { pageTitle } from './lib/shell/navigation';
  import { shell } from './lib/shell/shell-state.svelte';
  import AppShell from './lib/shell/AppShell.svelte';
  import RouteView from './lib/shell/RouteView.svelte';
  import {
    authMethods,
    bootstrapExternalSession,
    currentSession,
    type SessionInfo,
  } from './lib/auth';
  import { onboardingState, setupAvailable } from './lib/onboarding';
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
      void liveSeries.ensureResourcesSeeded((windowMs) =>
        fetchSparklines(
          ['cpu', 'memory'],
          windowMs <= 3_600_000
            ? '1h'
            : windowMs <= 3 * 3_600_000
              ? '3h'
              : '6h',
        ),
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
    liveSeries.reset();
    shell.stopIncidentPolling();
    session = null;
  }

  /**
   * The session ended while the app was open (expiry, sign-out elsewhere,
   * revoked token). Drop it and go to sign-in with a `next` back to the page
   * the user was on, instead of leaving an empty shell (UI-5).
   */
  function sessionLost() {
    if (!session) return;
    const here = location.pathname + location.search + location.hash;
    endSession();
    router.navigate(loginPath(here), { replace: true });
  }

  $effect(() => {
    if (session && live.state === 'unauthorized') sessionLost();
  });

  onMount(() => {
    const startedAtRoot = location.pathname === '/';
    prefs.bootstrap();
    router.install();
    const stopLinks = interceptLinks();
    // Any API call answered 401 may mean the session ended; confirm before
    // acting so one stray 401 (a revoked token page, say) does not sign out.
    const onSessionEnded = () => {
      if (!session) return;
      void sessionActive()
        .catch(() => true)
        .then((active) => {
          if (!active) sessionLost();
        });
    };
    addEventListener(sessionEndedEvent, onSessionEnded);

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
            router.navigate(homePath, { replace: true });
          }
          void prefs
            .load()
            .then((saved) => {
              if (startedAtRoot)
                router.navigate(landingPath(saved.landingPage), {
                  replace: true,
                });
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
          const here = location.pathname + location.search + location.hash;
          void setupAvailable().then((available) => {
            router.navigate(available ? '/setup' : loginPath(here), {
              replace: true,
            });
          });
        }
      });

    return () => {
      stopLinks();
      removeEventListener(sessionEndedEvent, onSessionEnded);
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
    router.navigate(homePath, { replace: true });
  }

  const title = $derived(pageTitle(router.name));
</script>

<svelte:head><title>Binnacle — {title}</title></svelte:head>

<a class="skip-link" href="#content">Skip to content</a>
{#if loading}
  <main class="access-state" aria-busy="true">
    <img src="/brand/binnacle-mark.svg" alt="" width="48" height="48" />
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
    <RouteView {live} onsignedout={signedOut} />
  </AppShell>
{/if}
