<script lang="ts">
  import {
    createChannel,
    updateChannel,
    type Channel,
    type ChannelInput,
    type ChannelKind,
  } from '../api/channels';
  import { errorMessage, isApiError } from '../api/client';
  import Button from '../ui/Button.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import Select from '../ui/Select.svelte';
  import Switch from '../ui/Switch.svelte';
  import { toasts } from '../ui/toast.svelte';

  let {
    open = $bindable(false),
    channel = null,
    onsaved,
  }: {
    open?: boolean;
    channel?: Channel | null;
    onsaved?: (channel: Channel) => void;
  } = $props();

  let name = $state('');
  let kind = $state<ChannelKind>('webhook');
  let url = $state('');
  let bearerToken = $state('');
  let signingSecret = $state('');
  let host = $state('');
  let sender = $state('');
  let recipients = $state('');
  let tlsMode = $state<'starttls' | 'implicit'>('starttls');
  let username = $state('');
  let password = $state('');
  let minimumSeverity = $state<'warning' | 'critical'>('warning');
  let notifyResolved = $state(true);
  let enabled = $state(true);
  let busy = $state(false);
  let error = $state('');
  let masterKeyMissing = $state(false);

  $effect(() => {
    if (!open) return;
    name = channel?.name ?? '';
    kind = channel?.kind ?? 'webhook';
    url = channel?.config.url ?? '';
    host = channel?.config.host ?? '';
    sender = channel?.config.sender ?? '';
    recipients = (channel?.config.recipients ?? []).join(', ');
    tlsMode = channel?.config.tlsMode ?? 'starttls';
    username = channel?.config.username ?? '';
    minimumSeverity = channel?.minimumSeverity ?? 'warning';
    notifyResolved = channel?.notifyResolved ?? true;
    enabled = channel?.enabled ?? true;
    bearerToken = '';
    signingSecret = '';
    password = '';
    error = '';
    masterKeyMissing = false;
  });

  async function save() {
    if (busy) return;
    error = '';
    if (!name.trim()) {
      error = 'Give the channel a name.';
      return;
    }
    const input: ChannelInput = {
      name: name.trim(),
      kind,
      enabled,
      minimumSeverity,
      notifyResolved,
    };
    if (kind === 'webhook') {
      if (!/^https:\/\//i.test(url.trim())) {
        error = 'Webhooks must use an https:// URL.';
        return;
      }
      input.url = url.trim();
      if (bearerToken) input.bearerToken = bearerToken;
      if (signingSecret) input.signingSecret = signingSecret;
    } else {
      const list = recipients
        .split(',')
        .map((value) => value.trim())
        .filter(Boolean);
      if (!host.trim() || !sender.trim() || !list.length) {
        error = 'SMTP needs a host, a sender, and at least one recipient.';
        return;
      }
      input.host = host.trim();
      input.sender = sender.trim();
      input.recipients = list;
      input.tlsMode = tlsMode;
      if (username) input.username = username;
      if (password) input.password = password;
    }
    busy = true;
    try {
      const saved = channel
        ? await updateChannel(channel.id, input)
        : await createChannel(input);
      toasts.success(channel ? 'Channel updated' : 'Channel created', {
        description: saved.name,
      });
      open = false;
      onsaved?.(saved);
    } catch (reason) {
      if (isApiError(reason, 'master_key_missing')) masterKeyMissing = true;
      else error = errorMessage(reason);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog
  bind:open
  title={channel ? 'Edit channel' : 'New notification channel'}
  description="Incidents are delivered when they open, update, and resolve, with reminders every two hours while open."
  size="md"
  dismissible={!busy}
>
  <form
    class="form"
    onsubmit={(event) => {
      event.preventDefault();
      void save();
    }}
  >
    <div class="row">
      <Field label="Name" required>
        {#snippet children({ id })}
          <Input
            {id}
            bind:value={name}
            maxlength={120}
            placeholder="Ops channel"
            data-autofocus
          />
        {/snippet}
      </Field>
      <Field label="Type">
        {#snippet children({ id })}
          <Select {id} bind:value={kind} disabled={Boolean(channel)}>
            <option value="webhook">HTTPS webhook</option>
            <option value="smtp">Email (SMTP)</option>
          </Select>
        {/snippet}
      </Field>
    </div>
    {#if kind === 'webhook'}
      <Field
        label="Webhook URL"
        required
        hint="Binnacle POSTs a JSON payload. HTTPS only."
      >
        {#snippet children({ id, describedBy })}
          <Input
            {id}
            type="url"
            bind:value={url}
            mono
            placeholder="https://hooks.example.com/binnacle"
            aria-describedby={describedBy}
          />
        {/snippet}
      </Field>
      <div class="row">
        <Field
          label="Bearer token"
          hint={channel?.secretConfigured
            ? 'Leave blank to keep the stored token.'
            : 'Optional Authorization header.'}
        >
          {#snippet children({ id, describedBy })}
            <Input
              {id}
              type="password"
              autocomplete="new-password"
              bind:value={bearerToken}
              mono
              aria-describedby={describedBy}
            />
          {/snippet}
        </Field>
        <Field
          label="Signing secret"
          hint="Optional HMAC-SHA256 signature of the payload."
        >
          {#snippet children({ id, describedBy })}
            <Input
              {id}
              type="password"
              autocomplete="new-password"
              bind:value={signingSecret}
              mono
              aria-describedby={describedBy}
            />
          {/snippet}
        </Field>
      </div>
    {:else}
      <div class="row">
        <Field label="SMTP host and port" required>
          {#snippet children({ id })}
            <Input
              {id}
              bind:value={host}
              mono
              placeholder="smtp.example.com:587"
            />
          {/snippet}
        </Field>
        <Field label="TLS">
          {#snippet children({ id })}
            <Select {id} bind:value={tlsMode}>
              <option value="starttls">STARTTLS</option>
              <option value="implicit">Implicit TLS</option>
            </Select>
          {/snippet}
        </Field>
      </div>
      <div class="row">
        <Field label="Sender" required>
          {#snippet children({ id })}
            <Input
              {id}
              type="email"
              bind:value={sender}
              placeholder="binnacle@example.com"
            />
          {/snippet}
        </Field>
        <Field label="Recipients" required hint="Comma-separated, up to 20.">
          {#snippet children({ id, describedBy })}
            <Input
              {id}
              bind:value={recipients}
              placeholder="ops@example.com, oncall@example.com"
              aria-describedby={describedBy}
            />
          {/snippet}
        </Field>
      </div>
      <div class="row">
        <Field label="Username" hint="Optional">
          {#snippet children({ id })}
            <Input {id} bind:value={username} autocomplete="username" />
          {/snippet}
        </Field>
        <Field
          label="Password"
          hint={channel?.secretConfigured
            ? 'Leave blank to keep the stored password.'
            : 'Optional'}
        >
          {#snippet children({ id })}
            <Input
              {id}
              type="password"
              autocomplete="new-password"
              bind:value={password}
            />
          {/snippet}
        </Field>
      </div>
    {/if}
    <div class="row">
      <Field label="Notify for">
        {#snippet children({ id })}
          <Select {id} bind:value={minimumSeverity}>
            <option value="warning">Warnings and critical</option>
            <option value="critical">Critical only</option>
          </Select>
        {/snippet}
      </Field>
      <div class="switches">
        <Field label="Notify when resolved" inline>
          {#snippet children({ id })}
            <Switch {id} bind:checked={notifyResolved} />
          {/snippet}
        </Field>
        <Field label="Enabled" inline>
          {#snippet children({ id })}
            <Switch {id} bind:checked={enabled} />
          {/snippet}
        </Field>
      </div>
    </div>
    {#if masterKeyMissing}
      <div class="callout" role="alert">
        <strong>A master key is required to store channel secrets.</strong>
        <p>
          Set <code>BINNACLE_MASTER_KEY</code> or
          <code>BINNACLE_MASTER_KEY_FILE</code> in the deployment and restart Binnacle,
          then try again.
        </p>
      </div>
    {:else if error}
      <p class="error" role="alert">{error}</p>
    {/if}
  </form>
  {#snippet footer()}
    <Button variant="ghost" onclick={() => (open = false)} disabled={busy}
      >Cancel</Button
    >
    <Button variant="primary" onclick={save} loading={busy}
      >{channel ? 'Save channel' : 'Create channel'}</Button
    >
  {/snippet}
</Dialog>

<style>
  .form {
    display: grid;
    gap: var(--space-4);
  }
  .row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-3);
    align-items: start;
  }
  .switches {
    display: grid;
    gap: var(--space-2);
    padding-top: 26px;
  }
  .error {
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  .callout {
    display: grid;
    gap: var(--space-1);
    padding: var(--space-3);
    border: 1px solid var(--warn-border);
    border-radius: var(--radius-sm);
    background: var(--warn-bg);
    font-size: var(--text-sm);
  }
  .callout strong {
    color: var(--warn-fg);
  }
  .callout code {
    font-size: var(--text-xs);
  }
  @media (max-width: 600px) {
    .row {
      grid-template-columns: 1fr;
    }
    .switches {
      padding-top: 0;
    }
  }
</style>
