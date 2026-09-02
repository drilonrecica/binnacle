<script lang="ts">
  import type { Snippet } from 'svelte';
  import Button from './Button.svelte';
  import Dialog from './Dialog.svelte';
  import { errorMessage } from '../api/client';

  let {
    open = $bindable(false),
    title,
    description,
    confirmLabel = 'Confirm',
    cancelLabel = 'Cancel',
    tone = 'danger',
    /** Text the user must type before the action is enabled. */
    phrase,
    onconfirm,
    children,
  }: {
    open?: boolean;
    title: string;
    description?: string;
    confirmLabel?: string;
    cancelLabel?: string;
    tone?: 'danger' | 'primary';
    phrase?: string;
    onconfirm: () => Promise<void> | void;
    children?: Snippet;
  } = $props();

  let typed = $state('');
  let busy = $state(false);
  let error = $state('');

  const ready = $derived(!phrase || typed.trim() === phrase);

  async function confirm() {
    if (!ready || busy) return;
    busy = true;
    error = '';
    try {
      await onconfirm();
      open = false;
    } catch (reason) {
      error = errorMessage(reason, 'The action could not be completed.');
    } finally {
      busy = false;
    }
  }

  $effect(() => {
    if (!open) {
      typed = '';
      error = '';
    }
  });
</script>

<Dialog bind:open {title} {description} size="sm" dismissible={!busy}>
  {#if children}<div class="content">{@render children()}</div>{/if}
  {#if phrase}
    <label class="phrase">
      <span>Type <code>{phrase}</code> to continue</span>
      <input
        type="text"
        autocomplete="off"
        spellcheck="false"
        bind:value={typed}
        data-autofocus
        onkeydown={(event) => {
          if (event.key === 'Enter') void confirm();
        }}
      />
    </label>
  {/if}
  {#if error}<p class="error" role="alert">{error}</p>{/if}
  {#snippet footer()}
    <Button variant="ghost" onclick={() => (open = false)} disabled={busy}
      >{cancelLabel}</Button
    >
    <Button
      variant={tone}
      onclick={confirm}
      disabled={!ready}
      loading={busy}
      data-autofocus={phrase ? undefined : true}
    >
      {confirmLabel}
    </Button>
  {/snippet}
</Dialog>

<style>
  .content {
    color: var(--text-2);
    font-size: var(--text-sm);
  }
  .phrase {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-4);
    font-size: var(--text-sm);
    color: var(--text-2);
  }
  .phrase code {
    padding: 1px 5px;
    border-radius: 4px;
    background: var(--surface-2);
    color: var(--text);
  }
  .phrase input {
    height: var(--control-h);
    padding: 0 var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    color: var(--text);
    font-family: var(--font-mono);
  }
  .error {
    margin-top: var(--space-3);
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
</style>
