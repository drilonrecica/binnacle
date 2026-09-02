<script lang="ts">
  import { tick } from 'svelte';
  import { claimSetup, verifySetupToken } from '../onboarding';
  import { errorMessage } from '../api/client';
  import AuthShell from '../auth/AuthShell.svelte';
  import Button from '../ui/Button.svelte';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';

  let { onclaimed }: { onclaimed: () => void } = $props();

  let step = $state<1 | 2>(1);
  let token = $state('');
  let username = $state('admin');
  let password = $state('');
  let confirmation = $state('');
  let busy = $state(false);
  let error = $state('');
  let errorElement = $state<HTMLElement | null>(null);

  const passwordProblems = $derived.by(() => {
    const problems: string[] = [];
    if (password.length < 12) problems.push('at least 12 characters');
    if (password.length > 128) problems.push('at most 128 characters');
    return problems;
  });

  async function fail(reason: unknown) {
    error = errorMessage(reason, 'Setup failed.');
    await tick();
    errorElement?.focus();
  }

  async function verify(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    error = '';
    try {
      await verifySetupToken(token.trim());
      step = 2;
    } catch (reason) {
      await fail(reason);
    } finally {
      busy = false;
    }
  }

  async function claim(event: SubmitEvent) {
    event.preventDefault();
    if (passwordProblems.length) {
      await fail(
        new Error(`The password needs ${passwordProblems.join(' and ')}.`),
      );
      return;
    }
    if (password !== confirmation) {
      await fail(new Error('The passwords do not match.'));
      return;
    }
    busy = true;
    error = '';
    try {
      await claimSetup(token.trim(), username.trim(), password);
      onclaimed();
    } catch (reason) {
      await fail(reason);
    } finally {
      busy = false;
    }
  }
</script>

<AuthShell
  eyebrow={`Step ${step} of 2`}
  title={step === 1 ? 'Set up Binnacle' : 'Create the administrator'}
  description={step === 1
    ? 'Enter the one-time setup token from the deployment. It is shown in the container logs or the BINNACLE_SETUP_TOKEN variable.'
    : 'This is the only account. It signs in locally with a password; two-factor can be added later.'}
>
  <ol class="steps" aria-label="Setup progress">
    <li class:done={step > 1} class:current={step === 1}>Verify token</li>
    <li class:current={step === 2}>Administrator</li>
  </ol>
  {#if step === 1}
    <form class="form" onsubmit={verify} aria-busy={busy}>
      <Field label="Setup token" required>
        {#snippet children({ id })}
          <Input
            {id}
            type="password"
            autocomplete="one-time-code"
            bind:value={token}
            required
            mono
            data-autofocus
          />
        {/snippet}
      </Field>
      {#if error}<p
          class="error"
          role="alert"
          tabindex="-1"
          bind:this={errorElement}
        >
          {error}
        </p>{/if}
      <Button type="submit" variant="primary" loading={busy}
        >Verify token</Button
      >
    </form>
  {:else}
    <form class="form" onsubmit={claim} aria-busy={busy}>
      <Field label="Username" required>
        {#snippet children({ id })}
          <Input
            {id}
            autocomplete="username"
            bind:value={username}
            required
            data-autofocus
          />
        {/snippet}
      </Field>
      <Field
        label="Password"
        required
        hint={password
          ? passwordProblems.length
            ? `Needs ${passwordProblems.join(' and ')}.`
            : 'Looks good.'
          : '12 to 128 characters. A passphrase works well.'}
      >
        {#snippet children({ id, describedBy })}
          <Input
            {id}
            type="password"
            autocomplete="new-password"
            bind:value={password}
            required
            minlength={12}
            maxlength={128}
            aria-describedby={describedBy}
          />
        {/snippet}
      </Field>
      <Field
        label="Confirm password"
        required
        error={confirmation && confirmation !== password
          ? 'The passwords do not match.'
          : undefined}
      >
        {#snippet children({ id, invalid })}
          <Input
            {id}
            type="password"
            autocomplete="new-password"
            bind:value={confirmation}
            required
            {invalid}
          />
        {/snippet}
      </Field>
      {#if error}<p
          class="error"
          role="alert"
          tabindex="-1"
          bind:this={errorElement}
        >
          {error}
        </p>{/if}
      <div class="actions">
        <Button variant="ghost" onclick={() => (step = 1)} disabled={busy}
          >Back</Button
        >
        <Button type="submit" variant="primary" loading={busy}
          >Create administrator</Button
        >
      </div>
    </form>
  {/if}
</AuthShell>

<style>
  .steps {
    display: flex;
    gap: var(--space-4);
    margin: 0;
    padding: 0;
    list-style: none;
    counter-reset: step;
  }
  .steps li {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-3);
    font-size: var(--text-xs);
    font-weight: 500;
  }
  .steps li::before {
    counter-increment: step;
    content: counter(step);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    border: 1px solid var(--border-strong);
    border-radius: 50%;
    font-family: var(--font-mono);
  }
  .steps li.current {
    color: var(--text);
  }
  .steps li.current::before {
    border-color: var(--accent);
    background: var(--accent);
    color: var(--accent-contrast);
  }
  .steps li.done::before {
    content: '✓';
    border-color: var(--ok-solid);
    color: var(--ok-fg);
  }
  .form {
    display: grid;
    gap: var(--space-4);
  }
  .actions {
    display: flex;
    justify-content: space-between;
    gap: var(--space-2);
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
