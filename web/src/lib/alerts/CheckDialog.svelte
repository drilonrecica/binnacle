<script lang="ts">
  import { untrack } from 'svelte';
  import type { LiveSnapshot } from '../live.svelte';
  import {
    checkToInput,
    createCheck,
    updateCheck,
    type Check,
    type CheckInput,
  } from '../api/checks';
  import { errorMessage } from '../api/client';
  import ResourcePicker from '../resources/ResourcePicker.svelte';
  import Button from '../ui/Button.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import Select from '../ui/Select.svelte';
  import Switch from '../ui/Switch.svelte';
  import { toasts } from '../ui/toast.svelte';

  let {
    open = $bindable(false),
    snapshot = null,
    check = null,
    resourceId: presetResource = '',
    onsaved,
  }: {
    open?: boolean;
    snapshot?: LiveSnapshot | null;
    /** Existing check to edit; null creates a new one. */
    check?: Check | null;
    resourceId?: string;
    onsaved?: (check: Check) => void;
  } = $props();

  const defaults = (): CheckInput => ({
    resourceId: presetResource,
    name: '',
    url: '',
    method: 'GET',
    intervalSeconds: 30,
    timeoutSeconds: 5,
    expectedStatusMin: 200,
    expectedStatusMax: 399,
    bodySubstring: '',
    required: true,
    enabled: true,
  });

  let form = $state<CheckInput>(defaults());
  let interval = $state('30');
  let timeout = $state('5');
  let statusMin = $state('200');
  let statusMax = $state('399');
  let busy = $state(false);
  let error = $state('');
  let fieldErrors = $state<Record<string, string>>({});

  $effect(() => {
    if (!open) return;
    const next = check ? checkToInput(check) : defaults();
    untrack(() => {
      form = next;
      interval = String(next.intervalSeconds);
      timeout = String(next.timeoutSeconds);
      statusMin = String(next.expectedStatusMin);
      statusMax = String(next.expectedStatusMax);
      error = '';
      fieldErrors = {};
    });
  });

  function validate(): CheckInput | null {
    const errors: Record<string, string> = {};
    const intervalSeconds = Number(interval);
    const timeoutSeconds = Number(timeout);
    const min = Number(statusMin);
    const max = Number(statusMax);
    if (!form.resourceId)
      errors.resourceId = 'Choose the resource this check belongs to.';
    if (!form.name.trim()) errors.name = 'Give the check a name.';
    if (!/^https?:\/\//i.test(form.url.trim()))
      errors.url = 'Enter an http:// or https:// URL.';
    else if (/^https?:\/\/(localhost|127\.|\[::1\])/i.test(form.url.trim()))
      errors.url =
        'Checks cannot target localhost. Use the container or public address.';
    if (
      !Number.isInteger(intervalSeconds) ||
      intervalSeconds < 10 ||
      intervalSeconds > 3600
    )
      errors.interval = 'Between 10 seconds and 1 hour.';
    if (
      !Number.isInteger(timeoutSeconds) ||
      timeoutSeconds < 1 ||
      timeoutSeconds > 30
    )
      errors.timeout = 'Between 1 and 30 seconds.';
    if (
      !Number.isInteger(min) ||
      !Number.isInteger(max) ||
      min < 100 ||
      max > 599 ||
      min > max
    )
      errors.status = 'Enter a valid status range such as 200–399.';
    if ((form.bodySubstring ?? '').length > 256)
      errors.body = 'At most 256 characters.';
    fieldErrors = errors;
    if (Object.keys(errors).length) return null;
    return {
      ...form,
      name: form.name.trim(),
      url: form.url.trim(),
      intervalSeconds,
      timeoutSeconds,
      expectedStatusMin: min,
      expectedStatusMax: max,
      bodySubstring: form.bodySubstring?.trim() ?? '',
    };
  }

  async function save() {
    if (busy) return;
    error = '';
    const input = validate();
    if (!input) return;
    busy = true;
    try {
      const saved = check
        ? await updateCheck(check.id, input)
        : await createCheck(input);
      toasts.success(check ? 'Check updated' : 'Check created', {
        description: `${saved.name} runs every ${input.intervalSeconds}s.`,
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
  title={check ? 'Edit health check' : 'New health check'}
  description="Binnacle requests the URL on a schedule. Required checks mark the resource down when they fail; optional checks mark it degraded."
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
    <Field label="Resource" required error={fieldErrors.resourceId}>
      {#snippet children({ id, invalid })}
        <ResourcePicker
          {id}
          {snapshot}
          bind:value={form.resourceId}
          {invalid}
          required
          includeInfrastructure={false}
        />
      {/snippet}
    </Field>
    <Field label="Name" required error={fieldErrors.name}>
      {#snippet children({ id, invalid })}
        <Input
          {id}
          bind:value={form.name}
          {invalid}
          maxlength={120}
          placeholder="Public homepage"
          data-autofocus={check ? undefined : true}
        />
      {/snippet}
    </Field>
    <div class="row url">
      <Field label="Method">
        {#snippet children({ id })}
          <Select {id} bind:value={form.method}>
            <option value="GET">GET</option>
            <option value="HEAD">HEAD</option>
          </Select>
        {/snippet}
      </Field>
      <Field
        label="URL"
        required
        error={fieldErrors.url}
        hint="HTTP or HTTPS. Private and loopback targets are blocked unless enabled in the deployment."
      >
        {#snippet children({ id, invalid, describedBy })}
          <Input
            {id}
            type="url"
            bind:value={form.url}
            {invalid}
            mono
            placeholder="https://example.com/health"
            aria-describedby={describedBy}
          />
        {/snippet}
      </Field>
    </div>
    <div class="row four">
      <Field label="Every" hint="Seconds" error={fieldErrors.interval}>
        {#snippet children({ id, invalid, describedBy })}
          <Input
            {id}
            type="number"
            inputmode="numeric"
            bind:value={interval}
            {invalid}
            mono
            aria-describedby={describedBy}
          />
        {/snippet}
      </Field>
      <Field label="Timeout" hint="Seconds" error={fieldErrors.timeout}>
        {#snippet children({ id, invalid, describedBy })}
          <Input
            {id}
            type="number"
            inputmode="numeric"
            bind:value={timeout}
            {invalid}
            mono
            aria-describedby={describedBy}
          />
        {/snippet}
      </Field>
      <Field label="Status from" error={fieldErrors.status}>
        {#snippet children({ id, invalid })}
          <Input
            {id}
            type="number"
            inputmode="numeric"
            bind:value={statusMin}
            {invalid}
            mono
          />
        {/snippet}
      </Field>
      <Field label="Status to">
        {#snippet children({ id })}
          <Input
            {id}
            type="number"
            inputmode="numeric"
            bind:value={statusMax}
            invalid={Boolean(fieldErrors.status)}
            mono
          />
        {/snippet}
      </Field>
    </div>
    <Field
      label="Body must contain"
      hint="Optional literal text that must appear in the response body."
      error={fieldErrors.body}
    >
      {#snippet children({ id, invalid, describedBy })}
        <Input
          {id}
          bind:value={form.bodySubstring}
          {invalid}
          maxlength={256}
          placeholder="&quot;status&quot;:&quot;ok&quot;"
          mono
          aria-describedby={describedBy}
        />
      {/snippet}
    </Field>
    <div class="switches">
      <Field
        label="Required"
        hint="Failures mark the resource down instead of degraded."
        inline
      >
        {#snippet children({ id })}
          <Switch {id} bind:checked={form.required} />
        {/snippet}
      </Field>
      <Field
        label="Enabled"
        hint="Disabled checks keep their settings but do not run."
        inline
      >
        {#snippet children({ id })}
          <Switch {id} bind:checked={form.enabled} />
        {/snippet}
      </Field>
    </div>
    {#if error}<p class="error" role="alert">{error}</p>{/if}
  </form>
  {#snippet footer()}
    <Button variant="ghost" onclick={() => (open = false)} disabled={busy}
      >Cancel</Button
    >
    <Button variant="primary" onclick={save} loading={busy}
      >{check ? 'Save check' : 'Create check'}</Button
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
    gap: var(--space-3);
  }
  .url {
    grid-template-columns: 110px 1fr;
  }
  .four {
    grid-template-columns: repeat(4, 1fr);
  }
  .switches {
    display: grid;
    gap: var(--space-3);
  }
  .error {
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  @media (max-width: 600px) {
    .url,
    .four {
      grid-template-columns: 1fr 1fr;
    }
  }
</style>
