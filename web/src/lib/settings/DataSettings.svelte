<script lang="ts">
  import type { LiveSnapshot } from '../live.svelte';
  import { retentionPresets, type SettingsSnapshot } from '../api/settings';
  import { monitorHealth, previewDeletion } from '../api/access';
  import { errorMessage } from '../api/client';
  import { formatAbsolute } from '../ui/relative-time';
  import ConfirmDialog from '../ui/ConfirmDialog.svelte';
  import { formatBytes } from '../i18n';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import Scale from '../ui/Scale.svelte';
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
  import { onMount, untrack } from 'svelte';
  import {
    effectiveTiers,
    shortening,
    tierKeys,
    tierLabels,
  } from './retention';

  let {
    snapshot,
    apply,
    live = null,
  }: {
    snapshot: SettingsSnapshot | null;
    apply: (changes: Record<string, string>) => Promise<SettingsSnapshot>;
    live?: LiveSnapshot | null;
  } = $props();

  const retention = new SettingsForm({
    keys: ['retention.preset', ...tierKeys],
    snapshot: () => snapshot,
    apply: (changes) => apply(changes),
    validators: Object.fromEntries(
      tierKeys.map((key) => [
        key,
        (value: string) =>
          parseGoDuration(value) == null && value !== '0' && value !== '0s'
            ? 'Use a duration such as 48h or 720h.'
            : null,
      ]),
    ),
  });

  const budget = new SettingsForm({
    keys: ['database.target_budget_bytes'],
    snapshot: () => snapshot,
    apply: (changes) => apply(changes),
    validators: {
      'database.target_budget_bytes': (value: string) =>
        /^\d+$/.test(value) && Number(value) >= 64 * 1024 * 1024
          ? null
          : 'At least 64 MiB.',
    },
  });

  /** What a shortening save would remove, shown before it is confirmed. */
  let pendingCut = $state<{
    tiers: string[];
    cutoff: Date;
    rows: number;
  } | null>(null);
  let confirmOpen = $state(false);
  let previewing = $state(false);

  /**
   * Saves retention, but when any tier would keep less history than today,
   * first asks the server how many rows are older than the new cutoff and
   * waits for confirmation (UI-8). A failed preview never saves.
   */
  async function saveRetention() {
    if (!retention.dirty || retention.saving || previewing) return;
    if (!retention.validate()) return;
    const current: Record<string, string> = {};
    for (const [key, setting] of Object.entries(snapshot?.values ?? {}))
      current[key] = setting.value;
    const cut = shortening(
      effectiveTiers(current),
      effectiveTiers(retention.values),
    );
    if (!cut) {
      await retention.save('Retention saved');
      return;
    }
    previewing = true;
    retention.error = '';
    try {
      const cutoff = new Date(Date.now() - cut.keepMs);
      const preview = await previewDeletion({
        kind: 'before',
        before: cutoff.toISOString(),
      });
      pendingCut = {
        tiers: cut.tiers.map((key) => tierLabels[key]),
        cutoff,
        rows: preview.totalRows,
      };
      confirmOpen = true;
    } catch (reason) {
      retention.error = `Nothing was saved: the deletion preview failed. ${errorMessage(reason)}`;
    } finally {
      previewing = false;
    }
  }

  let budgetValue = $state('1');
  let budgetUnit = $state<ByteUnit>('GiB');
  let databaseBytes = $state<number | null>(null);

  $effect(() => {
    if (!snapshot) return;
    retention.sync();
    if (untrack(() => budget.dirty)) return;
    budget.sync();
    const current = Number(
      snapshot.values['database.target_budget_bytes']?.value ?? 0,
    );
    const input = bytesToInput(current);
    budgetValue = String(input.value);
    budgetUnit = input.unit;
  });

  function syncBudget() {
    const bytes = inputToBytes(Number(budgetValue), budgetUnit);
    budget.values['database.target_budget_bytes'] =
      bytes == null ? '' : String(bytes);
  }

  const preset = $derived(retention.values['retention.preset'] ?? 'balanced');
  const budgetBytes = $derived(
    Number(budget.values['database.target_budget_bytes'] ?? 0),
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
    dirty={retention.dirty}
    saving={retention.saving || previewing}
    error={retention.error}
    onsave={() => void saveRetention()}
    onreset={() => retention.reset()}
  >
    <RadioCards
      label="Retention preset"
      bind:value={retention.values['retention.preset']}
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
            hint={retention.values[key] === '0' ||
            retention.values[key] === '0s'
              ? 'Tier disabled'
              : humanDuration(retention.values[key] ?? '')}
            error={retention.errors[key]}
          >
            {#snippet children({ id, invalid, describedBy })}
              <Input
                {id}
                bind:value={retention.values[key]}
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
    dirty={budget.dirty}
    saving={budget.saving}
    error={budget.error}
    onsave={() => void budget.save('Storage budget saved')}
    onreset={() => {
      budget.reset();
      const input = bytesToInput(budgetBytes);
      budgetValue = String(input.value);
      budgetUnit = input.unit;
    }}
  >
    <div class="budget">
      <Field
        label="Database budget"
        error={budget.errors['database.target_budget_bytes']}
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
        <Scale value={usagePct} label="Database usage" />
      </div>
    {/if}
  </SettingsSection>

  <HistoryDeletionCard snapshot={live} />
</div>

<ConfirmDialog
  bind:open={confirmOpen}
  title="Shorten retention?"
  description="Keeping less history deletes what falls outside the new limits. The next retention pass removes it, and it cannot be brought back."
  confirmLabel="Shorten and delete"
  onconfirm={() => retention.save('Retention saved')}
>
  {#if pendingCut}
    <p class="cut">
      Shorter: {pendingCut.tiers.join(', ')}.
    </p>
    <p class="cut">
      Up to <strong class="num">{pendingCut.rows.toLocaleString()}</strong>
      rows are older than
      <span class="num">{formatAbsolute(pendingCut.cutoff)}</span>
      today. Tiers that keep longer hold on to theirs.
    </p>
  {/if}
</ConfirmDialog>

<style>
  .stack {
    display: grid;
    gap: var(--space-4);
  }
  .tiers {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    color: var(--muted);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }
  .tiers b {
    color: var(--muted);
    font-weight: 500;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }
  .cut {
    font-size: var(--text-meta);
  }
  .hint {
    color: var(--muted);
    font-size: var(--text-mono);
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
    color: var(--muted);
    font-size: var(--text-meta);
  }
  @media (max-width: 720px) {
    .grid,
    .budget {
      grid-template-columns: 1fr;
    }
  }
</style>
