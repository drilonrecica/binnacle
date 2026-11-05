<script lang="ts">
  import KeyRound from '@lucide/svelte/icons/key-round';
  import { changePassword } from '../api/access';
  import { errorMessage } from '../api/client';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import { toasts } from '../ui/toast.svelte';

  const minimumLength = 12;

  let open = $state(false);
  let current = $state('');
  let next = $state('');
  let confirmation = $state('');
  let busy = $state(false);
  let error = $state('');

  const tooShort = $derived(
    next.length > 0 && [...next].length < minimumLength,
  );
  const mismatch = $derived(confirmation.length > 0 && confirmation !== next);

  function reset() {
    current = '';
    next = '';
    confirmation = '';
    error = '';
  }

  async function submit() {
    if (tooShort || mismatch || !current || !next) return;
    busy = true;
    error = '';
    try {
      await changePassword(current, next);
      toasts.success('Password changed', {
        description: 'Other sessions were signed out.',
      });
      open = false;
      reset();
    } catch (reason) {
      error = errorMessage(reason);
    } finally {
      busy = false;
    }
  }
</script>

<Card
  title="Password"
  description="Changing the password signs out every other session. This one stays signed in."
>
  <div class="actions">
    <Button
      onclick={() => {
        reset();
        open = true;
      }}
    >
      {#snippet icon()}<KeyRound />{/snippet}
      Change password…
    </Button>
  </div>
</Card>

<Dialog
  bind:open
  title="Change password"
  description="Confirm the current password, then choose a new one of at least 12 characters."
  size="sm"
  dismissible={!busy}
  onclose={reset}
>
  <form
    class="form"
    onsubmit={(event) => {
      event.preventDefault();
      void submit();
    }}
  >
    <Field label="Current password" required>
      {#snippet children({ id })}
        <Input
          {id}
          type="password"
          autocomplete="current-password"
          bind:value={current}
          required
          data-autofocus
        />
      {/snippet}
    </Field>
    <Field
      label="New password"
      required
      error={tooShort ? `At least ${minimumLength} characters.` : undefined}
    >
      {#snippet children({ id, invalid, describedBy })}
        <Input
          {id}
          type="password"
          autocomplete="new-password"
          bind:value={next}
          {invalid}
          aria-describedby={describedBy}
          required
        />
      {/snippet}
    </Field>
    <Field
      label="Confirm new password"
      required
      error={mismatch ? 'The passwords do not match.' : undefined}
    >
      {#snippet children({ id, invalid, describedBy })}
        <Input
          {id}
          type="password"
          autocomplete="new-password"
          bind:value={confirmation}
          {invalid}
          aria-describedby={describedBy}
          required
        />
      {/snippet}
    </Field>
    {#if error}<p class="error" role="alert">{error}</p>{/if}
  </form>
  {#snippet footer()}
    <Button variant="quiet" onclick={() => (open = false)} disabled={busy}
      >Cancel</Button
    >
    <Button
      variant="primary"
      onclick={submit}
      loading={busy}
      disabled={tooShort || mismatch || !current || !next || !confirmation}
      >Change password</Button
    >
  {/snippet}
</Dialog>

<style>
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
    font-size: var(--text-meta);
  }
</style>
