<script lang="ts">
  import { onMount } from 'svelte';
  import type { SettingsSnapshot } from '../api/settings';
  import { authMethods, type AuthMethods } from '../auth';
  import Badge from '../ui/Badge.svelte';
  import Card from '../ui/Card.svelte';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import MfaCard from './MfaCard.svelte';
  import SettingMeta from './SettingMeta.svelte';
  import SettingsSection from './SettingsSection.svelte';
  import { SettingsForm } from './settings-form.svelte';
  import { humanDuration, parseGoDuration } from './units';

  let {
    snapshot,
    apply,
    onsignedout,
  }: {
    snapshot: SettingsSnapshot | null;
    apply: (changes: Record<string, string>) => Promise<SettingsSnapshot>;
    onsignedout: () => void;
  } = $props();

  let methods = $state<AuthMethods | null>(null);

  const form = new SettingsForm({
    keys: ['sessions.idle_timeout', 'sessions.absolute_lifetime'],
    snapshot: () => snapshot,
    apply: (changes) => apply(changes),
    validators: {
      'sessions.idle_timeout': (value) => {
        const ms = parseGoDuration(value);
        return ms == null || ms < 300_000 || ms > 30 * 86_400_000
          ? 'Between 5 minutes and 30 days.'
          : null;
      },
      'sessions.absolute_lifetime': (value) => {
        const ms = parseGoDuration(value);
        return ms == null || ms < 3_600_000 || ms > 365 * 86_400_000
          ? 'Between 1 hour and 365 days.'
          : null;
      },
    },
  });

  $effect(() => {
    if (snapshot) form.reset();
  });

  onMount(() => {
    void authMethods()
      .then((value) => (methods = value))
      .catch(() => (methods = null));
  });
</script>

<div class="stack">
  <SettingsSection
    title="Sessions"
    description="Browser sessions slide with activity up to the idle timeout and never outlive the absolute lifetime."
    dirty={form.dirty}
    saving={form.saving}
    error={form.error}
    onsave={() => void form.save('Session settings saved')}
    onreset={() => form.reset()}
  >
    <div class="row">
      <Field
        label="Idle timeout"
        hint={`Sign out after ${humanDuration(form.values['sessions.idle_timeout'] ?? '')} without activity.`}
        error={form.errors['sessions.idle_timeout']}
      >
        {#snippet children({ id, invalid, describedBy })}
          <Input
            {id}
            bind:value={form.values['sessions.idle_timeout']}
            {invalid}
            mono
            aria-describedby={describedBy}
          />
        {/snippet}
      </Field>
      <SettingMeta setting={snapshot?.values['sessions.idle_timeout']} />
    </div>
    <div class="row">
      <Field
        label="Absolute lifetime"
        hint={`Sessions end ${humanDuration(form.values['sessions.absolute_lifetime'] ?? '')} after sign-in regardless of activity.`}
        error={form.errors['sessions.absolute_lifetime']}
      >
        {#snippet children({ id, invalid, describedBy })}
          <Input
            {id}
            bind:value={form.values['sessions.absolute_lifetime']}
            {invalid}
            mono
            aria-describedby={describedBy}
          />
        {/snippet}
      </Field>
      <SettingMeta setting={snapshot?.values['sessions.absolute_lifetime']} />
    </div>
  </SettingsSection>

  <MfaCard available={Boolean(snapshot?.features.advancedAuth)} {onsignedout} />

  <Card
    title="Sign-in methods"
    description="Binnacle has one local administrator. A trusted reverse proxy can supply the identity instead."
  >
    {#snippet actions()}
      {#if methods}<Badge tone="neutral" mono>{methods.mode}</Badge>{/if}
    {/snippet}
    <dl class="methods">
      <div>
        <dt>Local password</dt>
        <dd>
          {methods
            ? methods.local
              ? 'Enabled'
              : 'Disabled by the proxy mode'
            : '—'}
        </dd>
      </div>
      <div>
        <dt>Trusted proxy identity</dt>
        <dd>
          {#if !snapshot?.features.advancedAuth}
            Requires advanced authentication in the deployment.
          {:else if methods?.proxy}
            Configured{methods.proxyAvailable
              ? ' and reachable from this request'
              : ''}.
          {:else}
            Not configured. Set the auth mode, identity header, allowed subject,
            and proxy CIDRs in the deployment.
          {/if}
        </dd>
      </div>
    </dl>
  </Card>
</div>

<style>
  .stack {
    display: grid;
    gap: var(--space-5);
  }
  .row {
    display: grid;
    grid-template-columns: minmax(0, 360px) auto;
    gap: var(--space-4);
    align-items: end;
  }
  .methods {
    display: grid;
    gap: var(--space-3);
    margin: 0;
    font-size: var(--text-sm);
  }
  .methods div {
    display: grid;
    gap: 2px;
  }
  dt {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  dd {
    margin: 0;
  }
  @media (max-width: 720px) {
    .row {
      grid-template-columns: 1fr;
    }
  }
</style>
