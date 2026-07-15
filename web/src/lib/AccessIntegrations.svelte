<script lang="ts">
  import { onMount } from 'svelte';
  import { authenticatedMutation, authMethods } from './auth';

  type CoolifyStatus = {
    enabled: boolean;
    url?: string;
    tokenConfigured: boolean;
    environmentAuthoritative: boolean;
    collector: {
      state: string;
      lastSuccessAt?: string;
      errorCode?: string;
      resources: number;
    };
  };
  type Enrollment = { seed: string; uri: string; expiresAt: string };
  let coolify = $state<CoolifyStatus | null>(null);
  let url = $state('');
  let token = $state('');
  let password = $state('');
  let code = $state('');
  let mfaEnabled = $state(false);
  let mfaAvailable = $state(false);
  let enrollment = $state<Enrollment | null>(null);
  let recoveryCodes = $state<string[]>([]);
  let busy = $state('');
  let message = $state('');
  let error = $state('');

  onMount(() => {
    void fetch('/api/v1/integrations/coolify').then(async (r) => {
      if (r.ok) {
        coolify = await r.json();
        url = coolify?.url ?? '';
      }
    });
    void authMethods()
      .then(async (methods) => {
        mfaAvailable = methods.mfaAvailable;
        if (!mfaAvailable) return;
        const response = await fetch('/api/v1/auth/mfa');
        if (response.ok) mfaEnabled = (await response.json()).enabled;
      })
      .catch(() => {
        mfaAvailable = false;
      });
  });
  async function action(name: string, run: () => Promise<void>) {
    busy = name;
    message = '';
    error = '';
    try {
      await run();
    } catch (reason) {
      error = reason instanceof Error ? reason.message : 'Request failed.';
    } finally {
      busy = '';
    }
  }
  function saveCoolify() {
    return action('coolify', async () => {
      coolify = await authenticatedMutation<CoolifyStatus>(
        '/api/v1/integrations/coolify',
        'PUT',
        { URL: url, Token: token },
      );
      token = '';
      message = 'Coolify configuration saved.';
    });
  }
  function testCoolify() {
    return action('test', async () => {
      await authenticatedMutation('/api/v1/integrations/coolify/test', 'POST', {
        URL: url,
        Token: token,
      });
      message = 'Coolify read access verified.';
    });
  }
  function enroll() {
    return action('enroll', async () => {
      enrollment = await authenticatedMutation<Enrollment>(
        '/api/v1/auth/mfa/enroll',
        'POST',
        { password },
      );
      password = '';
      message = 'Enter a code from your authenticator to confirm.';
    });
  }
  function confirm() {
    return action('confirm', async () => {
      const result = await authenticatedMutation<{ recoveryCodes: string[] }>(
        '/api/v1/auth/mfa/confirm',
        'POST',
        { code },
      );
      recoveryCodes = result?.recoveryCodes ?? [];
      enrollment = null;
      code = '';
      mfaEnabled = true;
      message =
        'MFA enabled. Save every recovery code now; your other sessions were revoked.';
    });
  }
  function disable() {
    return action('disable', async () => {
      await authenticatedMutation('/api/v1/auth/mfa/disable', 'POST', {
        password,
        code,
      });
      location.assign('/login');
    });
  }
</script>

{#if error}<p role="alert">{error}</p>{/if}{#if message}<p role="status">
    {message}
  </p>{/if}
{#if mfaAvailable}<h3>Local MFA</h3>
  {#if recoveryCodes.length}
    <p>These one-time recovery codes will not be shown again.</p>
    <ul class="technical-value">
      {#each recoveryCodes as recovery}<li>{recovery}</li>{/each}
    </ul>
  {:else if enrollment}
    <p>Manual Base32 seed: <code>{enrollment.seed}</code></p>
    <p class="technical-value">{enrollment.uri}</p>
    <label for="mfa-confirm">Six-digit code</label><input
      id="mfa-confirm"
      autocomplete="one-time-code"
      inputmode="numeric"
      bind:value={code}
      maxlength="6"
    />
    <button onclick={confirm} disabled={busy !== ''}>Confirm MFA</button>
  {:else if mfaEnabled}
    <p>
      MFA is enabled for local authentication. The upstream provider owns MFA
      for external authentication.
    </p>
    <label for="mfa-disable-password">Current password</label><input
      id="mfa-disable-password"
      type="password"
      autocomplete="current-password"
      bind:value={password}
    />
    <label for="mfa-disable-code">Authentication or recovery code</label><input
      id="mfa-disable-code"
      autocomplete="one-time-code"
      bind:value={code}
    />
    <button onclick={disable} disabled={busy !== ''}>Disable MFA</button>
  {:else}
    <label for="mfa-password">Current password</label><input
      id="mfa-password"
      type="password"
      autocomplete="current-password"
      bind:value={password}
    />
    <button onclick={enroll} disabled={busy !== ''}>Set up MFA</button>
  {/if}{/if}

<h3>Coolify enrichment</h3>
{#if coolify}
  <p>
    Collector: <strong>{coolify.collector.state}</strong> · {coolify.collector
      .resources} resources{#if coolify.collector.errorCode}
      · {coolify.collector.errorCode}{/if}
  </p>
  <label for="coolify-url">Coolify URL</label><input
    id="coolify-url"
    type="url"
    bind:value={url}
    disabled={coolify.environmentAuthoritative}
    placeholder="https://coolify.example.test"
  />
  <label for="coolify-token"
    >Team-scoped read token {coolify.tokenConfigured
      ? '(leave blank to keep current)'
      : ''}</label
  ><input
    id="coolify-token"
    type="password"
    autocomplete="new-password"
    bind:value={token}
    disabled={coolify.environmentAuthoritative}
  />
  {#if coolify.environmentAuthoritative}<p>
      Deployment environment configuration is authoritative.
    </p>{:else}<button onclick={testCoolify} disabled={busy !== ''}
      >Test read access</button
    ><button onclick={saveCoolify} disabled={busy !== ''}>Save Coolify</button
    >{/if}
  <p>
    Only safe metadata is retained. Compose files, environment values, API logs,
    and secrets are excluded.
  </p>
{:else}<p role="status">Loading Coolify integration status…</p>{/if}
