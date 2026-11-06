<script lang="ts">
  import { onMount } from 'svelte';
  import FlaskConical from '@lucide/svelte/icons/flask-conical';
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import {
    completeOnboarding,
    diagnosticDisplay,
    diagnosticSummary,
    onboardingState,
    runDiagnostics,
    saveOnboarding,
    type OnboardingState,
  } from '../onboarding';
  import { retentionPresets } from '../api/settings';
  import { errorMessage } from '../api/client';
  import AuthShell from '../auth/AuthShell.svelte';
  import Steps from '../auth/Steps.svelte';
  import Button from '../ui/Button.svelte';
  import RadioCards from '../ui/RadioCards.svelte';
  import Status from '../ui/Status.svelte';
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
    diagnostics.filter((item) => item.status === 'failed' && !item.synthetic),
  );
  const requiredFailures = $derived(failures.filter((item) => item.required));
  const summary = $derived(diagnosticSummary(diagnostics));

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
        : 'Binnacle is ready. The checklist on Headroom points at the remaining optional setup.'}
>
  <Steps label="Onboarding progress" {steps} current={step} />

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
      {#if summary.synthetic}
        <div class="notice info" role="note">
          <FlaskConical aria-hidden="true" />
          <div>
            <strong>Demo mode: host and Docker data are synthetic.</strong>
            <p>
              Those checks show “synthetic data” because demo mode generates
              them instead of reading this server. Storage and the database are
              checked for real.
            </p>
          </div>
        </div>
      {/if}
      <ul class="checks">
        {#each diagnostics as check (check.id)}
          {@const display = diagnosticDisplay(check)}
          <li data-status={check.status} data-synthetic={check.synthetic}>
            <div class="check-head">
              <span class="check-title"
                >{check.name}{#if check.required}<span class="required">
                    · required</span
                  >{/if}</span
              >
              <Status tone={display.tone} label={display.label} size="sm" />
            </div>
            <span class="check-reason">{check.reason}</span>
            {#if check.suggestedFix}<span class="check-fix"
                >{check.suggestedFix}</span
              >{/if}
            {#if check.technicalDetail}<details>
                <summary>Technical detail</summary><code
                  >{check.technicalDetail}</code
                >
              </details>{/if}
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
          >{summary.passed} of {summary.total - summary.synthetic} passed</strong
        >{#if summary.toReview}, {summary.toReview} to review{/if}{#if summary.synthetic};
          {summary.synthetic}
          on synthetic data (demo mode){/if}
      </li>
      <li>
        Next: add a notification channel and a health check when you are ready.
      </li>
    </ul>
  {/if}

  {#if error}<p class="error" role="alert">{error}</p>{/if}

  <div class="actions">
    {#if step > 1}
      <Button variant="quiet" onclick={() => (step -= 1)} disabled={busy}
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
  .notice {
    display: flex;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--warn-border);
    border-radius: var(--radius-card);
    background: var(--warn-bg);
    font-size: var(--text-meta);
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
  .notice.info {
    border-color: var(--info-border);
    background: var(--info-bg);
  }
  .notice.info :global(svg),
  .notice.info strong {
    color: var(--info-fg);
  }
  .notice p {
    margin-top: 2px;
    color: var(--muted);
  }
  .facts {
    display: grid;
    gap: var(--space-2);
    margin: 0;
    padding-left: var(--space-4);
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .tiers {
    color: var(--muted);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }
  .outbound {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    font-size: var(--text-meta);
  }
  .muted {
    color: var(--muted);
  }
  .running {
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .checks {
    display: grid;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .checks li {
    display: grid;
    gap: 2px;
    padding: var(--space-3);
    border: 1px solid var(--line);
    border-radius: var(--radius-control);
    background: var(--panel-2);
    font-size: var(--text-meta);
  }
  .checks li[data-status='failed']:not([data-synthetic='true']) {
    border-color: var(--critical-border);
  }
  .check-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-1) var(--space-3);
  }
  .check-title {
    font-weight: var(--weight-semibold);
  }
  .required {
    color: var(--muted);
    font-weight: var(--weight-regular);
    font-size: var(--text-mono);
  }
  .check-reason {
    color: var(--muted);
  }
  .check-fix {
    color: var(--warn-fg);
  }
  details {
    font-size: var(--text-mono);
  }
  details code {
    display: block;
    margin-top: var(--space-1);
    overflow-wrap: anywhere;
  }
  .warn {
    color: var(--warn-fg);
    font-size: var(--text-meta);
  }
  .error {
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--critical-border);
    border-radius: var(--radius-control);
    background: var(--critical-bg);
    color: var(--critical-fg);
    font-size: var(--text-meta);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }
  .actions-right {
    display: flex;
    flex-wrap: wrap;
    margin-left: auto;
    gap: var(--space-2);
  }
</style>
