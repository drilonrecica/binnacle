<script lang="ts">
  import type { SettingsSnapshot } from '../api/settings';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import SettingMeta from './SettingMeta.svelte';
  import SettingsSection from './SettingsSection.svelte';
  import { SettingsForm } from './settings-form.svelte';
  import { humanDuration, parseGoDuration } from './units';

  let {
    snapshot,
    apply,
  }: {
    snapshot: SettingsSnapshot | null;
    apply: (changes: Record<string, string>) => Promise<SettingsSnapshot>;
  } = $props();

  function duration(min: number, max: number) {
    return (value: string) => {
      const ms = parseGoDuration(value);
      if (ms == null) return 'Use a duration such as 5s, 2m, or 1h.';
      if (ms < min || ms > max)
        return `Between ${humanDuration(`${min}ms`)} and ${humanDuration(`${max}ms`)}.`;
      return null;
    };
  }

  const form = new SettingsForm({
    keys: [
      'collection.host_interval',
      'collection.container_interval',
      'persistence.raw_interval',
      'charts.max_points_per_series',
    ],
    snapshot: () => snapshot,
    apply: (changes) => apply(changes),
    validators: {
      'collection.host_interval': duration(1000, 300_000),
      'collection.container_interval': duration(1000, 300_000),
      'persistence.raw_interval': duration(5000, 600_000),
      'charts.max_points_per_series': (value) =>
        /^\d+$/.test(value) && Number(value) >= 100 && Number(value) <= 5000
          ? null
          : 'Between 100 and 5000 points.',
    },
  });

  $effect(() => {
    if (snapshot) form.reset();
  });

  function aggressive(key: string) {
    const ms = parseGoDuration(form.values[key] ?? '');
    return ms != null && ms < 2000;
  }
</script>

<div class="stack">
  <SettingsSection
    title="Collection"
    description="How often Binnacle samples the host and containers. Shorter intervals cost CPU and Docker API calls."
    dirty={form.dirty}
    saving={form.saving}
    error={form.error}
    onsave={() => void form.save('Collection settings saved')}
    onreset={() => form.reset()}
  >
    <div class="row">
      <Field
        label="Host interval"
        hint={`Currently ${humanDuration(form.values['collection.host_interval'] ?? '')}${aggressive('collection.host_interval') ? ' · below 2s adds noticeable load' : ''}`}
        error={form.errors['collection.host_interval']}
      >
        {#snippet children({ id, invalid, describedBy })}
          <Input
            {id}
            bind:value={form.values['collection.host_interval']}
            {invalid}
            mono
            aria-describedby={describedBy}
          />
        {/snippet}
      </Field>
      <SettingMeta setting={snapshot?.values['collection.host_interval']} />
    </div>
    <div class="row">
      <Field
        label="Container interval"
        hint={`Currently ${humanDuration(form.values['collection.container_interval'] ?? '')}${aggressive('collection.container_interval') ? ' · below 2s adds noticeable Docker API load' : ''}`}
        error={form.errors['collection.container_interval']}
      >
        {#snippet children({ id, invalid, describedBy })}
          <Input
            {id}
            bind:value={form.values['collection.container_interval']}
            {invalid}
            mono
            aria-describedby={describedBy}
          />
        {/snippet}
      </Field>
      <SettingMeta
        setting={snapshot?.values['collection.container_interval']}
      />
    </div>
    <div class="row">
      <Field
        label="Persist raw samples every"
        hint="Raw samples are written in batches at this interval."
        error={form.errors['persistence.raw_interval']}
      >
        {#snippet children({ id, invalid, describedBy })}
          <Input
            {id}
            bind:value={form.values['persistence.raw_interval']}
            {invalid}
            mono
            aria-describedby={describedBy}
          />
        {/snippet}
      </Field>
      <SettingMeta setting={snapshot?.values['persistence.raw_interval']} />
    </div>
    <div class="row">
      <Field
        label="Chart points per series"
        hint="Longer ranges are downsampled to this many points."
        error={form.errors['charts.max_points_per_series']}
      >
        {#snippet children({ id, invalid, describedBy })}
          <Input
            {id}
            type="number"
            inputmode="numeric"
            bind:value={form.values['charts.max_points_per_series']}
            {invalid}
            mono
            aria-describedby={describedBy}
          />
        {/snippet}
      </Field>
      <SettingMeta setting={snapshot?.values['charts.max_points_per_series']} />
    </div>
  </SettingsSection>
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
  @media (max-width: 720px) {
    .row {
      grid-template-columns: 1fr;
    }
  }
</style>
