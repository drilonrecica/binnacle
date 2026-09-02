<script lang="ts">
  import { onMount } from 'svelte';
  import Copy from '@lucide/svelte/icons/copy';
  import Download from '@lucide/svelte/icons/download';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import { confirmMfa, disableMfa, enrollMfa, mfaStatus } from '../api/access';
  import { errorMessage } from '../api/client';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import { toasts } from '../ui/toast.svelte';

  let {
    available,
    onsignedout,
  }: { available: boolean; onsignedout: () => void } = $props();

  let enabled = $state<boolean | null>(null);
  let enrollOpen = $state(false);
  let disableOpen = $state(false);
  let password = $state('');
  let code = $state('');
  let seed = $state('');
  let uri = $state('');
  let recoveryCodes = $state<string[]>([]);
  let busy = $state(false);
  let error = $state('');

  onMount(() => {
    if (!available) return;
    void mfaStatus()
      .then((value) => (enabled = value.enabled))
      .catch(() => (enabled = null));
  });

  function reset() {
    password = '';
    code = '';
    seed = '';
    uri = '';
    error = '';
  }

  async function enroll() {
    busy = true;
    error = '';
    try {
      const result = await enrollMfa(password);
      seed = result.seed;
      uri = result.uri;
      password = '';
    } catch (reason) {
      error = errorMessage(reason);
    } finally {
      busy = false;
    }
  }

  async function confirm() {
    busy = true;
    error = '';
    try {
      const result = await confirmMfa(code);
      recoveryCodes = result.recoveryCodes;
      enabled = true;
      code = '';
      seed = '';
      toasts.success('Two-factor authentication enabled', {
        description: 'Other sessions were signed out.',
      });
    } catch (reason) {
      error = errorMessage(reason);
    } finally {
      busy = false;
    }
  }

  async function disable() {
    busy = true;
    error = '';
    try {
      await disableMfa(password, code);
      toasts.success('Two-factor authentication disabled', {
        description: 'Sign in again to continue.',
      });
      disableOpen = false;
      onsignedout();
    } catch (reason) {
      error = errorMessage(reason);
    } finally {
      busy = false;
    }
  }

  async function copy(text: string, what: string) {
    try {
      await navigator.clipboard.writeText(text);
      toasts.success(`${what} copied`);
    } catch {
      toasts.error(`Could not copy ${what.toLowerCase()}`);
    }
  }

  function downloadCodes() {
    const blob = new Blob([recoveryCodes.join('\n') + '\n'], {
      type: 'text/plain',
    });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = 'binnacle-recovery-codes.txt';
    anchor.click();
    URL.revokeObjectURL(url);
  }
</script>

<Card
  title="Two-factor authentication"
  description="A time-based one-time code from an authenticator app is required at sign-in."
>
  {#snippet actions()}
    {#if !available}
      <Badge tone="neutral">Disabled at deployment</Badge>
    {:else if enabled}
      <Badge tone="ok" dot>Enabled</Badge>
    {:else if enabled === false}
      <Badge tone="neutral">Off</Badge>
    {/if}
  {/snippet}
  {#if !available}
    <p class="note">
      Advanced authentication is switched off in this deployment. Set <code
        >BINNACLE_FEATURE_ADVANCED_AUTH=true</code
      > and restart to enable TOTP and trusted-proxy sign-in.
    </p>
  {:else if recoveryCodes.length}
    <div class="recovery">
      <p>
        <strong>Save these recovery codes now.</strong> Each one works once and they
        will not be shown again.
      </p>
      <ol class="codes">
        {#each recoveryCodes as recovery (recovery)}<li>
            <code>{recovery}</code>
          </li>{/each}
      </ol>
      <div class="actions">
        <Button
          size="sm"
          onclick={() => copy(recoveryCodes.join('\n'), 'Recovery codes')}
          >{#snippet icon()}<Copy />{/snippet}Copy all</Button
        >
        <Button size="sm" onclick={downloadCodes}
          >{#snippet icon()}<Download />{/snippet}Download</Button
        >
        <Button size="sm" variant="ghost" onclick={() => (recoveryCodes = [])}
          >Done</Button
        >
      </div>
    </div>
  {:else if enabled}
    <div class="actions">
      <Button
        variant="danger"
        onclick={() => {
          reset();
          disableOpen = true;
        }}>Turn off two-factor…</Button
      >
    </div>
  {:else}
    <div class="actions">
      <Button
        variant="primary"
        onclick={() => {
          reset();
          enrollOpen = true;
        }}
      >
        {#snippet icon()}<ShieldCheck />{/snippet}
        Set up two-factor…
      </Button>
    </div>
  {/if}
</Card>

<Dialog
  bind:open={enrollOpen}
  title="Set up two-factor authentication"
  description={seed
    ? 'Add the secret to your authenticator, then enter the code it shows.'
    : 'Confirm your password to generate a secret.'}
  size="sm"
  dismissible={!busy}
  onclose={reset}
>
  <form
    class="form"
    onsubmit={(event) => {
      event.preventDefault();
      void (seed ? confirm() : enroll());
    }}
  >
    {#if !seed}
      <Field label="Password" required>
        {#snippet children({ id })}
          <Input
            {id}
            type="password"
            autocomplete="current-password"
            bind:value={password}
            required
            data-autofocus
          />
        {/snippet}
      </Field>
    {:else}
      <div class="secret">
        <span class="label">Secret</span>
        <div class="secret-row">
          <code>{seed}</code>
          <Button size="sm" variant="ghost" onclick={() => copy(seed, 'Secret')}
            >{#snippet icon()}<Copy />{/snippet}Copy</Button
          >
        </div>
        <span class="label">Setup URI</span>
        <div class="secret-row">
          <code class="uri">{uri}</code>
          <Button
            size="sm"
            variant="ghost"
            onclick={() => copy(uri, 'Setup URI')}
            >{#snippet icon()}<Copy />{/snippet}Copy</Button
          >
        </div>
      </div>
      <Field label="Code from your app" required>
        {#snippet children({ id })}
          <Input
            {id}
            inputmode="numeric"
            autocomplete="one-time-code"
            bind:value={code}
            required
            mono
            data-autofocus
          />
        {/snippet}
      </Field>
    {/if}
    {#if error}<p class="error" role="alert">{error}</p>{/if}
  </form>
  {#snippet footer()}
    <Button variant="ghost" onclick={() => (enrollOpen = false)} disabled={busy}
      >Cancel</Button
    >
    {#if !seed}
      <Button variant="primary" onclick={enroll} loading={busy}>Continue</Button
      >
    {:else}
      <Button variant="primary" onclick={confirm} loading={busy}>Enable</Button>
    {/if}
  {/snippet}
</Dialog>

<Dialog
  bind:open={disableOpen}
  title="Turn off two-factor authentication"
  description="Confirm with your password and a current code. Every session is signed out afterwards."
  size="sm"
  dismissible={!busy}
>
  <form
    class="form"
    onsubmit={(event) => {
      event.preventDefault();
      void disable();
    }}
  >
    <Field label="Password" required>
      {#snippet children({ id })}<Input
          {id}
          type="password"
          autocomplete="current-password"
          bind:value={password}
          required
          data-autofocus
        />{/snippet}
    </Field>
    <Field label="Code or recovery code" required>
      {#snippet children({ id })}<Input
          {id}
          autocomplete="one-time-code"
          bind:value={code}
          required
          mono
        />{/snippet}
    </Field>
    {#if error}<p class="error" role="alert">{error}</p>{/if}
  </form>
  {#snippet footer()}
    <Button
      variant="ghost"
      onclick={() => (disableOpen = false)}
      disabled={busy}>Cancel</Button
    >
    <Button variant="danger" onclick={disable} loading={busy}>Turn off</Button>
  {/snippet}
</Dialog>

<style>
  .note {
    color: var(--text-2);
    font-size: var(--text-sm);
  }
  .note code {
    font-size: var(--text-xs);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .form {
    display: grid;
    gap: var(--space-4);
  }
  .error {
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  .secret {
    display: grid;
    gap: var(--space-1);
  }
  .label {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .secret-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }
  .secret-row code {
    flex: 1;
    min-width: 0;
    padding: var(--space-2);
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    font-size: var(--text-xs);
    overflow-wrap: anywhere;
  }
  .recovery {
    display: grid;
    gap: var(--space-3);
  }
  .codes {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-1) var(--space-4);
    margin: 0;
    padding-left: var(--space-5);
    font-family: var(--font-mono);
  }
</style>
