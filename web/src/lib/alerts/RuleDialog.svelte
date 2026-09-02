<script lang="ts">
  import { updateRule, type Rule } from '../api/alerts';
  import { errorMessage } from '../api/client';
  import Button from '../ui/Button.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import Select from '../ui/Select.svelte';
  import { toasts } from '../ui/toast.svelte';
  import { ruleInfo } from './rule-catalog';

  let {
    open = $bindable(false),
    rule,
    onsaved,
  }: {
    open?: boolean;
    rule: Rule | null;
    onsaved?: (rule: Rule) => void;
  } = $props();

  let severity = $state<'warning' | 'critical'>('warning');
  let threshold = $state('');
  let recoveryThreshold = $state('');
  let triggerMinutes = $state('');
  let recoveryMinutes = $state('');
  let busy = $state(false);
  let error = $state('');

  const info = $derived(rule ? ruleInfo(rule.family, rule.name) : null);

  $effect(() => {
    if (open && rule) {
      severity = rule.severity;
      threshold = rule.threshold == null ? '' : String(rule.threshold);
      recoveryThreshold =
        rule.recoveryThreshold == null ? '' : String(rule.recoveryThreshold);
      triggerMinutes = String(Math.round(rule.triggerSeconds / 60));
      recoveryMinutes = String(Math.round(rule.recoverySeconds / 60));
      error = '';
    }
  });

  function number(
    value: string,
    label: string,
    { min, max }: { min: number; max: number },
  ) {
    const parsed = Number(value);
    if (!Number.isFinite(parsed) || parsed < min || parsed > max)
      throw new Error(`${label} must be between ${min} and ${max}.`);
    return parsed;
  }

  async function save() {
    if (!rule || !info || busy) return;
    error = '';
    try {
      const update: Parameters<typeof updateRule>[1] = { severity };
      if (info.editable.includes('threshold'))
        update.threshold = number(threshold, 'Threshold', {
          min: 0,
          max: info.unit === '%' ? 100 : 1000,
        });
      if (info.editable.includes('recoveryThreshold'))
        update.recoveryThreshold = number(
          recoveryThreshold,
          'Recovery threshold',
          { min: 0, max: info.unit === '%' ? 100 : 1000 },
        );
      if (info.editable.includes('triggerSeconds'))
        update.triggerSeconds =
          number(triggerMinutes, 'Trigger duration', { min: 0, max: 1440 }) *
          60;
      if (info.editable.includes('recoverySeconds'))
        update.recoverySeconds =
          number(recoveryMinutes, 'Recovery duration', { min: 0, max: 1440 }) *
          60;
      if (
        update.threshold != null &&
        update.recoveryThreshold != null &&
        update.recoveryThreshold > update.threshold
      )
        throw new Error(
          'The recovery threshold must not exceed the trigger threshold.',
        );
      busy = true;
      const saved = await updateRule(rule.id, update);
      toasts.success('Rule updated', {
        description: `${info.title} now uses the new thresholds.`,
      });
      open = false;
      onsaved?.(saved);
    } catch (reason) {
      error = errorMessage(reason);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog
  bind:open
  title={info ? `Edit ${info.title}` : 'Edit rule'}
  description={info?.description}
  size="sm"
  dismissible={!busy}
>
  {#if rule && info}
    <form
      class="form"
      onsubmit={(event) => {
        event.preventDefault();
        void save();
      }}
    >
      <Field label="Severity">
        {#snippet children({ id })}
          <Select {id} bind:value={severity}>
            <option value="warning">Warning</option>
            <option value="critical">Critical</option>
          </Select>
        {/snippet}
      </Field>
      {#if info.editable.includes('threshold')}
        <div class="row">
          <Field
            label={info.unit === 'events'
              ? 'Events in window'
              : 'Trigger above'}
            hint={info.unit === '%'
              ? 'Percent'
              : info.unit === 'events'
                ? `Within ${Math.round((rule.windowSeconds ?? 600) / 60)} minutes`
                : undefined}
          >
            {#snippet children({ id, describedBy })}
              <Input
                {id}
                type="number"
                inputmode="decimal"
                bind:value={threshold}
                mono
                aria-describedby={describedBy}
                data-autofocus
              />
            {/snippet}
          </Field>
          {#if info.editable.includes('recoveryThreshold')}
            <Field label="Recover below" hint="Percent">
              {#snippet children({ id, describedBy })}
                <Input
                  {id}
                  type="number"
                  inputmode="decimal"
                  bind:value={recoveryThreshold}
                  mono
                  aria-describedby={describedBy}
                />
              {/snippet}
            </Field>
          {/if}
        </div>
      {/if}
      <div class="row">
        {#if info.editable.includes('triggerSeconds')}
          <Field label="Must persist for" hint="Minutes before the alert fires">
            {#snippet children({ id, describedBy })}
              <Input
                {id}
                type="number"
                inputmode="numeric"
                bind:value={triggerMinutes}
                mono
                aria-describedby={describedBy}
              />
            {/snippet}
          </Field>
        {/if}
        {#if info.editable.includes('recoverySeconds')}
          <Field
            label="Recovery time"
            hint="Minutes below threshold before it resolves"
          >
            {#snippet children({ id, describedBy })}
              <Input
                {id}
                type="number"
                inputmode="numeric"
                bind:value={recoveryMinutes}
                mono
                aria-describedby={describedBy}
              />
            {/snippet}
          </Field>
        {/if}
      </div>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
    </form>
  {/if}
  {#snippet footer()}
    <Button variant="ghost" onclick={() => (open = false)} disabled={busy}
      >Cancel</Button
    >
    <Button variant="primary" onclick={save} loading={busy}>Save rule</Button>
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
  }
  .error {
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
</style>
