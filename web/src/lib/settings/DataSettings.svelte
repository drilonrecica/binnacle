<script lang="ts">
  import type { LiveSnapshot } from '../live.svelte';
  import { retentionPresets, type SettingsSnapshot } from '../api/settings';
  import { monitorHealth } from '../api/access';
  import { formatBytes } from '../i18n';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import ProgressBar from '../ui/ProgressBar.svelte';
  import RadioCards from '../ui/RadioCards.svelte';
  import Select from '../ui/Select.svelte';
  import HistoryDeletionCard from './HistoryDeletionCard.svelte';
  import SettingMeta from './SettingMeta.svelte';
  import SettingsSection from './SettingsSection.svelte';
  import { SettingsForm } from './settings-form.svelte';
  import {
    byteUnitOptions,
    bytesToInput,
    humanDuration,
    inputToBytes,
    parseGoDuration,
    type ByteUnit,
  } from './units';
  import { onMount } from 'svelte';

  let {
    snapshot,
    apply,
    live = null,
  }: {
    snapshot: SettingsSnapshot | null;
    apply: (changes: Record<string, string>) => Promise<SettingsSnapshot>;
    live?: LiveSnapshot | null;
  } = $props();

  const tierKeys = [
    'retention.raw',
    'retention.one_minute',
    'retention.fifteen_minute',
    'retention.one_hour',
  ];
  const tierLabels: Record<string, string> = {
    'retention.raw': 'Raw samples (10s)',
    'retention.one_minute': '1-minute rollups',
    'retention.fifteen_minute': '15-minute rollups',
    'retention.one_hour': 'Hourly rollups',
  };

  const form = new SettingsForm({
    keys: ['retention.preset', ...tierKeys, 'database.target_budget_bytes'],
    snapshot: () => snapshot,
    apply: (changes) => apply(changes),
    validators: Object.fromEntries([
      ...tierKeys.map((key) => [
        key,
        (value: string) =>
          parseGoDuration(value) == null && value !== '0' && value !== '0s'
            ? 'Use a duration such as 48h or 720h.'
            : null,
      ]),
      [
        'database.target_budget_bytes',
        (value: string) =>
          /^\d+$/.test(value) && Number(value) >= 64 * 1024 * 1024
            ? null
            : 'At least 64 MiB.',
      ],
    ]),
  });

  let budgetValue = $state('1');
  let budgetUnit = $state<ByteUnit>('GiB');
  let databaseBytes = $state<number | null>(null);

  $effect(() => {
    if (!snapshot) return;
    form.reset();
    const current = Number(
      snapshot.values['database.target_budget_bytes']?.value ?? 0,
    );
    const input = bytesToInput(current);
    budgetValue = String(input.value);
    budgetUnit = input.unit;
  });

  function syncBudget() {
    const bytes = inputToBytes(Number(budgetValue), budgetUnit);
    form.values['database.target_budget_bytes'] =
      bytes == null ? '' : String(bytes);
  }

  const preset = $derived(form.values['retention.preset'] ?? 'balanced');
  const budgetBytes = $derived(
    Number(form.values['database.target_budget_bytes'] ?? 0),
  );
  const usagePct = $derived(
    databaseBytes != null && budgetBytes > 0
      ? (databaseBytes / budgetBytes) * 100
      : null,
  );

  onMount(() => {
    void monitorHealth()
      .then((value) => {
        const metric = value.metrics.find((item) => item.id === 'database');
        databaseBytes =
          metric && typeof metric.value === 'number' ? metric.value : null;
      })
      .catch(() => {
        /* Usage is informational. */
      });
  });
</script>

{#snippet tiers(value: string)}
  {@const found = retentionPresets.find((item) => item.value === value)}
  {#if found?.tiers}
    <span class="tiers">
      <span>raw <b>{found.tiers.raw}</b></span>
      <span>1m <b>{found.tiers.one_minute}</b></span>
      <span>15m <b>{found.tiers.fifteen_minute}</b></span>
      <span>1h <b>{found.tiers.one_hour}</b></span>
    </span>
  {/if}
{/snippet}

<div class="stack">
  <SettingsSection
    title="Retention"
    description="How long each tier of history stays on disk. Expired data is removed on a schedule; in-retention history is never removed silently."
    dirty={form.dirty}
    saving={form.saving}
    error={form.error}
    onsave={() => void form.save('Retention saved')}
    onreset={() => form.reset()}
  >
    <RadioCards
      label="Retention preset"
      bind:value={form.values['retention.preset']}
      options={retentionPresets.map((item) => ({
        value: item.value,
        title: item.title,
        description: item.description,
      }))}
      columns={2}
      extra={tiers}
    />
    <SettingMeta setting={snapshot?.values['retention.preset']} />
    {#if preset === 'advanced'}
      <div class="grid">
        {#each tierKeys as key (key)}
          <Field
            label={tierLabels[key]}
            hint={form.values[key] === '0' || form.values[key] === '0s'
              ? 'Tier disabled'
              : humanDuration(form.values[key] ?? '')}
            error={form.errors[key]}
          >
            {#snippet children({ id, invalid, describedBy })}
              <Input
                {id}
                bind:value={form.values[key]}
                {invalid}
                mono
                aria-describedby={describedBy}
              />
            {/snippet}
          </Field>
        {/each}
      </div>
      <p class="hint">
        Each tier must keep data longer than the one before it.
      </p>
    {/if}
  </SettingsSection>

  <SettingsSection
    title="Storage budget"
    description="Binnacle reports pressure at 80% and 95% of this budget and pauses raw persistence at 98% so the disk never fills."
    dirty={form.dirty}
    saving={form.saving}
    error={form.error}
    onsave={() => void form.save('Storage budget saved')}
    onreset={() => form.reset()}
  >
    <div class="budget">
      <Field
        label="Database budget"
        error={form.errors['database.target_budget_bytes']}
      >
        {#snippet children({ id, invalid })}
          <div class="budget-input">
            <Input
              {id}
              type="number"
              inputmode="numeric"
              bind:value={budgetValue}
              {invalid}
              mono
              oninput={syncBudget}
            />
            <Select
              bind:value={budgetUnit}
              aria-label="Unit"
              onchange={syncBudget}
            >
              {#each byteUnitOptions as unit (unit)}<option value={unit}
                  >{unit}</option
                >{/each}
            </Select>
          </div>
        {/snippet}
      </Field>
      <SettingMeta setting={snapshot?.values['database.target_budget_bytes']} />
    </div>
    {#if databaseBytes != null}
      <div class="usage">
        <div class="usage-head">
          <span>Current database size</span>
          <span class="num"
            >{formatBytes(databaseBytes)}
            {#if budgetBytes}of {formatBytes(budgetBytes)}{/if}</span
          >
        </div>
        <ProgressBar value={usagePct} label="Database usage" />
      </div>
    {/if}
  </SettingsSection>

  <HistoryDeletionCard snapshot={live} />
</div>

<style>
  .stack {
    display: grid;
    gap: var(--space-5);
  }
  .tiers {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .tiers b {
    color: var(--text-2);
    font-weight: 500;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }
  .hint {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .budget {
    display: grid;
    grid-template-columns: minmax(0, 360px) auto;
    gap: var(--space-4);
    align-items: end;
  }
  .budget-input {
    display: grid;
    grid-template-columns: 1fr 100px;
    gap: var(--space-2);
  }
  .usage {
    display: grid;
    gap: var(--space-2);
    max-width: 520px;
  }
  .usage-head {
    display: flex;
    justify-content: space-between;
    color: var(--text-2);
    font-size: var(--text-sm);
  }
  @media (max-width: 720px) {
    .grid,
    .budget {
      grid-template-columns: 1fr;
    }
  }
</style>
