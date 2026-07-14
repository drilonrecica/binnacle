<script lang="ts">
  import { onMount, tick } from 'svelte';
  import {
    AuthError,
    authMethods,
    bootstrapExternalSession,
    login,
    safeRedirect,
    type AuthMethods,
  } from './auth';
  import { SvelteURLSearchParams } from 'svelte/reactivity';
  let { onauthenticated }: { onauthenticated: (path: string) => void } =
    $props();
  let username = $state('');
  let password = $state('');
  let code = $state('');
  let methods = $state<AuthMethods | null>(null);
  let error = $state('');
  let busy = $state(false);
  let errorElement = $state<HTMLElement>();
  let usernameElement = $state<HTMLInputElement>();
  onMount(() => {
    void authMethods()
      .then((value) => {
        methods = value;
        if (value.local) usernameElement?.focus();
      })
      .catch(() => usernameElement?.focus());
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    error = '';
    try {
      await login(username, password, code);
      onauthenticated(
        safeRedirect(new SvelteURLSearchParams(location.search).get('next')),
      );
    } catch (reason) {
      const authError = reason as AuthError;
      error = authError.retryAfterSeconds
        ? `${authError.message} Retry in ${authError.retryAfterSeconds} seconds.`
        : authError.message;
      await tick();
      errorElement?.focus();
    } finally {
      busy = false;
    }
  }
  async function external() {
    busy = true;
    error = '';
    try {
      await bootstrapExternalSession();
      onauthenticated('/watch');
    } catch (reason) {
      error = (reason as AuthError).message;
    } finally {
      busy = false;
    }
  }
</script>

<section class="access-gate" aria-labelledby="login-title">
  <div class="access-brand" aria-hidden="true">
    <img src="/brand/binnacle-mark-dark.png" alt="" /><span
      >BINNACLE / ACCESS</span
    ><strong>LOCAL ADMINISTRATOR GATE</strong><small
      >SELF-HOSTED · READ-ONLY MONITOR</small
    >
  </div>
  <div class="access-form">
    <span class="eyebrow">AUTHENTICATION / 01</span>
    <h1 id="login-title">Sign in to Binnacle</h1>
    <p>
      {methods?.local === false
        ? 'Authentication is managed by the trusted upstream proxy.'
        : 'Use the local administrator account for this server.'}
    </p>
    {#if error}<p bind:this={errorElement} tabindex="-1" role="alert">
        {error}
      </p>{/if}
    {#if methods?.proxy && methods.proxyAvailable}<button
        type="button"
        onclick={external}
        disabled={busy}>Continue with external access</button
      >{/if}
    {#if methods?.local !== false}<form onsubmit={submit} aria-busy={busy}>
        <label for="username">Username</label>
        <input
          bind:this={usernameElement}
          id="username"
          name="username"
          autocomplete="username"
          required
          bind:value={username}
        />
        <label for="code"
          >Authentication or recovery code <span>(when enabled)</span></label
        >
        <input
          id="code"
          name="code"
          inputmode="numeric"
          autocomplete="one-time-code"
          bind:value={code}
        />
        <label for="password">Password</label>
        <input
          id="password"
          name="password"
          type="password"
          autocomplete="current-password"
          required
          bind:value={password}
        />
        <button type="submit" disabled={busy}
          >{busy ? 'Signing in…' : 'Sign in'}</button
        >
      </form>{:else if !methods?.proxyAvailable}<p role="status">
        External identity was not supplied by a trusted proxy.
      </p>{/if}
  </div>
</section>
