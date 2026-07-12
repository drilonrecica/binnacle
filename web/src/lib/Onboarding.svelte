<script lang="ts">
  import {
    completeOnboarding,
    onboardingState,
    runDiagnostics,
    saveOnboarding,
    type OnboardingState,
  } from './onboarding';
  import Badge from './ui/Badge.svelte';
  let { oncomplete }: { oncomplete: () => void } = $props();
  let onboarding = $state<OnboardingState>({ checklistDismissed: false });
  let retention = $state('balanced');
  let outbound = $state(false);
  let busy = $state(false);
  let error = $state('');

  void onboardingState()
    .then((value) => {
      onboarding = value;
      retention = value.retentionPreset ?? 'balanced';
    })
    .catch((reason) => (error = String(reason)));

  async function diagnose() {
    busy = true;
    error = '';
    try {
      onboarding = (await saveOnboarding(retention)) ?? onboarding;
      onboarding = (await runDiagnostics(outbound)) ?? onboarding;
    } catch (reason) {
      error = reason instanceof Error ? reason.message : 'Diagnostics failed.';
    } finally {
      busy = false;
    }
  }
  async function finish() {
    busy = true;
    error = '';
    try {
      await completeOnboarding();
      oncomplete();
    } catch (reason) {
      error = reason instanceof Error ? reason.message : 'Onboarding failed.';
    } finally {
      busy = false;
    }
  }

  const retentionPresets = [
    {
      value: 'minimal',
      name: 'Minimal',
      summary: 'Lowest disk use',
      detail: 'Raw 12h · 1-minute 7d · 15-minute 90d · hourly 1y',
    },
    {
      value: 'balanced',
      name: 'Balanced',
      summary: 'Recommended for most installations',
      detail: 'Raw 48h · 1-minute 30d · 15-minute 1y',
    },
    {
      value: 'long-term',
      name: 'Long-term',
      summary: 'More history and disk use',
      detail: 'Raw 7d · 1-minute 90d · 15-minute 2y',
    },
  ];
</script>

<section class="onboarding" aria-labelledby="onboarding-title">
  <header class="commission-header">
    <div class="commission-identity" aria-hidden="true">
      <img class="brand-logo-dark" src="/brand/binnacle-mark-dark.png" alt="" />
      <img class="brand-logo-light" src="/brand/binnacle-mark.png" alt="" />
    </div>
    <div>
      <span>BINNACLE / COMMISSIONING</span>
      <h1 id="onboarding-title">Bring this server onto watch</h1>
      <p>Choose how much history to keep, then verify the installation.</p>
    </div>
    <ul class="commission-assurances" aria-label="Installation assurances">
      <li>Self-hosted</li>
      <li>Local metrics</li>
      <li>No product telemetry</li>
    </ul>
  </header>
  {#if error}<p role="alert">{error}</p>{/if}
  <section class="commission-step">
    <span class="step-number">01</span>
    <div>
      <h2>Secure access</h2>
      <p class="security-notice">
        <strong>Binnacle does not configure network exposure.</strong>
        Keep private installations behind a restricted network or VPN. If this service
        is internet-accessible, use an HTTPS reverse proxy and consider an additional
        access-control layer.
      </p>
    </div>
  </section>
  <section class="commission-step">
    <span class="step-number">02</span>
    <div>
      <fieldset class="retention-options">
        <legend>Retention</legend>
        <p class="field-note">
          Choose how long historical metrics remain available. You can change
          this later in Settings.
        </p>
        {#each retentionPresets as preset (preset.value)}
          <label class="retention-option">
            <input type="radio" bind:group={retention} value={preset.value} />
            <span
              ><strong>{preset.name}</strong><small>{preset.summary}</small
              ></span
            >
            <code>{preset.detail}</code>
          </label>
        {/each}
      </fieldset>
    </div>
  </section>
  <section class="commission-step">
    <span class="step-number">03</span>
    <div>
      <h2>Diagnostics</h2>
      <label
        ><input type="checkbox" bind:checked={outbound} /> Test outbound HTTPS (optional)</label
      >
      <button type="button" disabled={busy} onclick={diagnose}
        >Run installation diagnostics</button
      >
    </div>
  </section>
  {#if onboarding.diagnostics?.length}
    <section class="commission-step" aria-labelledby="diagnostics-title">
      <span class="step-number">04</span>
      <div>
        <h2 id="diagnostics-title">Diagnostics</h2>
        {#each onboarding.diagnostics as check (check.id)}
          <article class="diagnostic-result">
            <h3>{check.name}</h3>
            <Badge
              state={check.status === 'passed'
                ? 'healthy'
                : check.status === 'failed'
                  ? 'down'
                  : 'unknown'}>{check.status}</Badge
            >
            <p>{check.reason}</p>
            {#if check.suggestedFix}<p>{check.suggestedFix}</p>{/if}
            {#if check.technicalDetail}<details>
                <summary>Technical detail</summary><code
                  >{check.technicalDetail}</code
                >
              </details>{/if}
          </article>
        {/each}
        <button type="button" disabled={busy} onclick={finish}
          >Enter dashboard</button
        >
      </div>
    </section>
  {/if}
</section>
