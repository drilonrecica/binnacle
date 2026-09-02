<script lang="ts">
  import { onMount, tick } from 'svelte';
  import LogIn from '@lucide/svelte/icons/log-in';
  import {
    AuthError,
    authMethods,
    bootstrapExternalSession,
    login,
    safeRedirect,
    type AuthMethods,
  } from '../auth';
  import { router } from '../router.svelte';
  import AuthShell from '../auth/AuthShell.svelte';
  import Button from '../ui/Button.svelte';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';

  let { onauthenticated }: { onauthenticated: (path: string) => void } =
    $props();

  let username = $state('');
  let password = $state('');
  let code = $state('');
  let methods = $state<AuthMethods | null>(null);
  let error = $state('');
  let busy = $state(false);
  let retryAt = $state<number | null>(null);
  let now = $state(Date.now());
  let errorElement = $state<HTMLElement | null>(null);

  const retrySeconds = $derived(
    retryAt ? Math.max(0, Math.ceil((retryAt - now) / 1000)) : 0,
  );

  onMount(() => {
    void authMethods()
      .then((value) => (methods = value))
      .catch(
        () =>
          (methods = {
            mode: 'local',
            local: true,
            proxy: false,
            proxyAvailable: false,
            mfaAvailable: false,
          }),
      );
    const timer = window.setInterval(() => (now = Date.now()), 1000);
    return () => window.clearInterval(timer);
  });

  async function fail(reason: unknown) {
    const authError = reason as AuthError;
    error = authError?.message ?? 'Sign-in failed.';
    retryAt = authError?.retryAfterSeconds
      ? Date.now() + authError.retryAfterSeconds * 1000
      : null;
    await tick();
    errorElement?.focus();
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (busy || retrySeconds > 0) return;
    busy = true;
    error = '';
    try {
      await login(username, password, code);
      onauthenticated(safeRedirect(router.param('next') || null));
    } catch (reason) {
      await fail(reason);
    } finally {
      busy = false;
    }
  }

  async function external() {
    busy = true;
    error = '';
    try {
      await bootstrapExternalSession();
      onauthenticated(safeRedirect(router.param('next') || null));
    } catch (reason) {
      await fail(reason);
    } finally {
      busy = false;
    }
  }
</script>

<AuthShell
  title="Sign in"
  description={methods?.local === false
    ? 'This server signs you in through its reverse proxy.'
    : 'Use the local administrator account for this server.'}
>
  {#if methods?.proxy && methods.proxyAvailable}
    <Button
      variant={methods.local ? 'secondary' : 'primary'}
      onclick={external}
      loading={busy}
    >
      {#snippet icon()}<LogIn />{/snippet}
      Continue with external access
    </Button>
    {#if methods.local}<p class="or">or sign in locally</p>{/if}
  {/if}
  {#if methods?.local !== false}
    <form class="form" onsubmit={submit} aria-busy={busy}>
      <Field label="Username" required>
        {#snippet children({ id })}
          <Input
            {id}
            name="username"
            autocomplete="username"
            bind:value={username}
            required
            data-autofocus
          />
        {/snippet}
      </Field>
      <Field label="Password" required>
        {#snippet children({ id })}
          <Input
            {id}
            name="password"
            type="password"
            autocomplete="current-password"
            bind:value={password}
            required
          />
        {/snippet}
      </Field>
      {#if methods?.mfaAvailable}
        <Field
          label="Authentication code"
          hint="From your authenticator app, or a recovery code. Leave blank if two-factor is off."
        >
          {#snippet children({ id, describedBy })}
            <Input
              {id}
              name="code"
              inputmode="numeric"
              autocomplete="one-time-code"
              bind:value={code}
              mono
              aria-describedby={describedBy}
            />
          {/snippet}
        </Field>
      {/if}
      {#if error}
        <p class="error" role="alert" tabindex="-1" bind:this={errorElement}>
          {error}{#if retrySeconds > 0}
            Try again in {retrySeconds}s.{/if}
        </p>
      {/if}
      <Button
        type="submit"
        variant="primary"
        loading={busy}
        disabled={retrySeconds > 0}>Sign in</Button
      >
    </form>
  {:else if !methods?.proxyAvailable}
    <p class="error" role="status">
      The trusted proxy did not supply an identity for this request.
    </p>
  {/if}
  {#snippet aside()}
    Sign-in attempts are rate limited. Sessions end after inactivity and can be
    revoked from Settings.
  {/snippet}
</AuthShell>

<style>
  .form {
    display: grid;
    gap: var(--space-4);
  }
  .or {
    color: var(--text-3);
    font-size: var(--text-xs);
    text-align: center;
  }
  .error {
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--critical-border);
    border-radius: var(--radius-sm);
    background: var(--critical-bg);
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
</style>
