<script lang="ts">
  import { onMount } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import CircleCheck from '@lucide/svelte/icons/circle-check';
  import Circle from '@lucide/svelte/icons/circle';
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import {
    completeOnboarding,
    onboardingState,
    runDiagnostics,
    saveOnboarding,
    type OnboardingState,
  } from '../onboarding';
  import { retentionPresets } from '../api/settings';
  import { errorMessage } from '../api/client';
  import AuthShell from '../auth/AuthShell.svelte';
  import Button from '../ui/Button.svelte';
  import RadioCards from '../ui/RadioCards.svelte';
  import Switch from '../ui/Switch.svelte';

  let { oncomplete }: { oncomplete: () => void } = $props();

  let step = $state(1);
  let onboarding = $state<OnboardingState>({ checklistDismissed: false });
  let retention = $state<'minimal' | 'balanced' | 'long-term'>('balanced');
  let outbound = $state(false);
  let busy = $state(false);
  let error = $state('');

  const steps = ['Security', 'Retention', 'Diagnostics', 'Done'];
  const diagnostics = $derived(onboarding.diagnostics ?? []);
  const failures = $derived(
    diagnostics.filter((item) => item.status === 'failed'),
  );
  const requiredFailures = $derived(failures.filter((item) => item.required));

  onMount(() => {
    void onboardingState()
      .then((value) => {
        onboarding = value;
        retention = value.retentionPreset ?? 'balanced';
        if (value.diagnostics?.length) step = 3;
      })
      .catch((reason) => (error = errorMessage(reason)));
  });

  async function diagnose() {
    busy = true;
    error = '';
    try {
      onboarding = (await saveOnboarding(retention)) ?? onboarding;
      onboarding = (await runDiagnostics(outbound)) ?? onboarding;
    } catch (reason) {
      error = errorMessage(reason, 'Diagnostics failed.');
    } finally {
      busy = false;
    }
  }

  async function next() {
    error = '';
    if (step === 2) {
      step = 3;
      if (!diagnostics.length) await diagnose();
      return;
    }
    if (step === 3) {
      step = 4;
      return;
    }
    step += 1;
  }

  async function finish() {
    busy = true;
    error = '';
    try {
      await completeOnboarding();
      oncomplete();
    } catch (reason) {
      error = errorMessage(reason, 'Onboarding could not be completed.');
    } finally {
      busy = false;
    }
  }
</script>

{#snippet tiers(value: string)}
  {@const found = retentionPresets.find((item) => item.value === value)}
  {#if found?.tiers}
    <span class="tiers"
      >raw {found.tiers.raw} · 1m {found.tiers.one_minute} · 15m {found.tiers
        .fifteen_minute}{#if found.tiers.one_hour !== 'off'}
        · 1h {found.tiers.one_hour}{/if}</span
    >
  {/if}
{/snippet}

<AuthShell
  wide
  eyebrow={`Step ${step} of ${steps.length}`}
  title={step === 1
    ? 'Before you start'
    : step === 2
      ? 'How much history to keep'
      : step === 3
        ? 'Checking the installation'
        : 'You are set'}
  description={step === 1
    ? 'Binnacle observes this server and never changes it. A few things it cannot do for you.'
    : step === 2
      ? 'Change this later under Settings → Data.'
      : step === 3
        ? 'Each check explains what to fix if it fails. Required checks affect what Binnacle can see.'
        : 'The dashboard is ready. The checklist on the Overview points at the remaining optional setup.'}
>
  <ol class="steps" aria-label="Onboarding progress">
    {#each steps as label, index (label)}
      <li class:done={step > index + 1} class:current={step === index + 1}>
        {label}
      </li>
    {/each}
  </ol>

  {#if step === 1}
    <div class="notice">
      <ShieldAlert aria-hidden="true" />
      <div>
        <strong>Binnacle does not configure network exposure.</strong>
        <p>
          Keep private installations behind a restricted network or VPN. If this
          address is reachable from the internet, put an HTTPS reverse proxy in
          front and consider an extra access-control layer.
        </p>
      </div>
    </div>
    <ul class="facts">
      <li>Read-only: it never restarts, stops, or changes containers.</li>
      <li>
        Local-first: metrics, alerts, and settings stay in one SQLite file on
        this server.
      </li>
      <li>
        No telemetry: nothing is sent anywhere unless you add a notification
        channel.
      </li>
    </ul>
  {:else if step === 2}
    <RadioCards
      label="Retention preset"
      bind:value={retention}
      options={retentionPresets
        .filter((item) => item.value !== 'advanced')
        .map((item) => ({
          value: item.value as 'minimal' | 'balanced' | 'long-term',
          title: item.title,
          description: item.description,
        }))}
      extra={tiers}
    />
  {:else if step === 3}
    <label class="outbound">
      <Switch bind:checked={outbound} label="Also test outbound HTTPS" />
      <span
        >Also test outbound HTTPS <span class="muted"
          >(only needed for webhook or email notifications)</span
        ></span
      >
    </label>
    {#if busy && !diagnostics.length}
      <p class="running" role="status">Running checks…</p>
    {:else if diagnostics.length}
      <ul class="checks">
        {#each diagnostics as check (check.id)}
          <li class={check.status}>
            <span class="check-icon" aria-hidden="true">
              {#if check.status === 'passed'}<CircleCheck
                />{:else if check.status === 'failed'}<CircleAlert
                />{:else}<Circle />{/if}
            </span>
            <div class="check-text">
              <span class="check-title"
                >{check.name}{#if check.required}<span class="required">
                    · required</span
                  >{/if}</span
              >
              <span class="check-reason">{check.reason}</span>
              {#if check.suggestedFix}<span class="check-fix"
                  >{check.suggestedFix}</span
                >{/if}
              {#if check.technicalDetail}<details>
                  <summary>Technical detail</summary><code
                    >{check.technicalDetail}</code
                  >
                </details>{/if}
            </div>
            <span class="sr-only">{check.status.replaceAll('_', ' ')}</span>
          </li>
        {/each}
      </ul>
      {#if requiredFailures.length}
        <p class="warn">
          Required checks failed. You can continue now and fix them later;
          container data stays limited until then.
        </p>
      {/if}
    {/if}
  {:else}
    <ul class="facts">
      <li>
        Retention: <strong
          >{retentionPresets.find((item) => item.value === retention)
            ?.title}</strong
        >
      </li>
      <li>
        Diagnostics: <strong
          >{diagnostics.filter((item) => item.status === 'passed').length} of {diagnostics.length}
          passed</strong
        >{#if failures.length}, {failures.length} to review{/if}
      </li>
      <li>
        Next: add a notification channel and a health check when you are ready.
      </li>
    </ul>
  {/if}

  {#if error}<p class="error" role="alert">{error}</p>{/if}

  <div class="actions">
    {#if step > 1}
      <Button variant="ghost" onclick={() => (step -= 1)} disabled={busy}
        >Back</Button
      >
    {:else}
      <span></span>
    {/if}
    {#if step < 3}
      <Button variant="primary" onclick={next} loading={busy}
        >{step === 2 ? 'Save and run checks' : 'Continue'}</Button
      >
    {:else if step === 3}
      <div class="actions-right">
        <Button onclick={diagnose} loading={busy}>Run checks again</Button>
        <Button
          variant="primary"
          onclick={next}
          disabled={busy || !diagnostics.length}>Continue</Button
        >
      </div>
    {:else}
      <Button variant="primary" onclick={finish} loading={busy}
        >Open the dashboard</Button
      >
    {/if}
  </div>
</AuthShell>

<style>
  .steps {
    display: flex;
    flex-wrap: wrap;
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
  .notice {
    display: flex;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--warn-border);
    border-radius: var(--radius);
    background: var(--warn-bg);
    font-size: var(--text-sm);
  }
  .notice :global(svg) {
    flex: none;
    width: 18px;
    height: 18px;
    margin-top: 2px;
    color: var(--warn-fg);
  }
  .notice strong {
    color: var(--warn-fg);
  }
  .notice p {
    margin-top: 2px;
    color: var(--text-2);
  }
  .facts {
    display: grid;
    gap: var(--space-2);
    margin: 0;
    padding-left: var(--space-5);
    color: var(--text-2);
    font-size: var(--text-sm);
  }
  .tiers {
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .outbound {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    font-size: var(--text-sm);
  }
  .muted {
    color: var(--text-3);
  }
  .running {
    color: var(--text-2);
    font-size: var(--text-sm);
  }
  .checks {
    display: grid;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .checks li {
    display: flex;
    gap: var(--space-3);
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
  }
  .check-icon {
    display: inline-flex;
    margin-top: 2px;
    color: var(--text-3);
  }
  .check-icon :global(svg) {
    width: 16px;
    height: 16px;
  }
  li.passed .check-icon {
    color: var(--ok-fg);
  }
  li.failed .check-icon {
    color: var(--critical-fg);
  }
  li.warning .check-icon {
    color: var(--warn-fg);
  }
  .check-text {
    display: grid;
    gap: 2px;
    font-size: var(--text-sm);
  }
  .check-title {
    font-weight: 600;
  }
  .required {
    color: var(--text-3);
    font-weight: 400;
    font-size: var(--text-xs);
  }
  .check-reason {
    color: var(--text-2);
  }
  .check-fix {
    color: var(--warn-fg);
  }
  details {
    font-size: var(--text-xs);
  }
  details code {
    display: block;
    margin-top: var(--space-1);
    overflow-wrap: anywhere;
  }
  .warn {
    color: var(--warn-fg);
    font-size: var(--text-sm);
  }
  .error {
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--critical-border);
    border-radius: var(--radius-sm);
    background: var(--critical-bg);
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  .actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }
  .actions-right {
    display: flex;
    gap: var(--space-2);
  }
</style>
