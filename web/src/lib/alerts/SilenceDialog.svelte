<script lang="ts">
  import type { LiveSnapshot } from '../live.svelte';
  import {
    createSilence,
    silencePresets,
    type SilencePreset,
    type SilenceScope,
  } from '../api/silences';
  import { errorMessage } from '../api/client';
  import Button from '../ui/Button.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import Select from '../ui/Select.svelte';
  import { toasts } from '../ui/toast.svelte';

  let {
    open = $bindable(false),
    snapshot = null,
    scope: initialScope = 'server',
    scopeId: initialScopeId = '',
    rules = [],
    oncreated,
  }: {
    open?: boolean;
    snapshot?: LiveSnapshot | null;
    scope?: SilenceScope;
    scopeId?: string;
    /** Built-in alert rules for the rule scope. */
    rules?: Array<{ id: string; name: string }>;
    oncreated?: () => void;
  } = $props();

  let scope = $state<SilenceScope>('server');
  let scopeId = $state('');
  let preset = $state<SilencePreset>('1h');
  let customEnd = $state('');
  let reason = $state('');
  let busy = $state(false);
  let error = $state('');

  const resources = $derived(snapshot?.resources ?? []);
  const projects = $derived([
    ...new Set(resources.map((resource) => resource.project).filter(Boolean)),
  ] as string[]);

  $effect(() => {
    if (open) {
      scope = initialScope;
      scopeId = initialScopeId;
      preset = '1h';
      customEnd = '';
      reason = '';
      error = '';
    }
  });

  const scopeLabel = $derived(
    scope === 'resource'
      ? 'Resource'
      : scope === 'project'
        ? 'Project'
        : scope === 'rule'
          ? 'Rule'
          : '',
  );

  async function submit() {
    if (busy) return;
    error = '';
    if (scope !== 'server' && !scopeId) {
      error = `Choose a ${scopeLabel.toLowerCase()}.`;
      return;
    }
    if (preset === 'custom' && !customEnd) {
      error = 'Choose when the silence ends.';
      return;
    }
    busy = true;
    try {
      await createSilence({
        scopeType: scope,
        scopeId: scope === 'server' ? undefined : scopeId,
        reason,
        preset,
        customEnd:
          preset === 'custom' ? new Date(customEnd).toISOString() : undefined,
      });
      toasts.success('Silence created', {
        description:
          'Notifications for this scope are paused. Alerts keep evaluating.',
      });
      open = false;
      oncreated?.();
    } catch (reason) {
      error = errorMessage(reason);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog
  bind:open
  title="Silence notifications"
  description="Silences pause notifications for a scope. Alerts keep evaluating, so a still-failing condition notifies again when the silence ends."
  size="md"
  dismissible={!busy}
>
  <form
    class="form"
    onsubmit={(event) => {
      event.preventDefault();
      void submit();
    }}
  >
    <Field label="Scope">
      {#snippet children({ id })}
        <Select {id} bind:value={scope} onchange={() => (scopeId = '')}>
          <option value="server">Whole server</option>
          <option value="project">Project</option>
          <option value="resource">Resource</option>
          <option value="rule">Alert rule</option>
        </Select>
      {/snippet}
    </Field>
    {#if scope === 'resource'}
      <Field label="Resource" required>
        {#snippet children({ id })}
          <Select {id} bind:value={scopeId} required>
            <option value="">Choose a resource</option>
            {#each resources as resource (resource.id)}
              <option value={resource.id}
                >{resource.name}{resource.project
                  ? ` · ${resource.project}`
                  : ''}</option
              >
            {/each}
          </Select>
        {/snippet}
      </Field>
    {:else if scope === 'project'}
      <Field label="Project" required>
        {#snippet children({ id })}
          {#if projects.length}
            <Select {id} bind:value={scopeId} required>
              <option value="">Choose a project</option>
              {#each projects as project (project)}
                <option value={project}>{project}</option>
              {/each}
            </Select>
          {:else}
            <Input
              {id}
              bind:value={scopeId}
              placeholder="Project name"
              required
            />
          {/if}
        {/snippet}
      </Field>
    {:else if scope === 'rule'}
      <Field label="Alert rule" required>
        {#snippet children({ id })}
          {#if rules.length}
            <Select {id} bind:value={scopeId} required>
              <option value="">Choose a rule</option>
              {#each rules as rule (rule.id)}
                <option value={rule.id}>{rule.name}</option>
              {/each}
            </Select>
          {:else}
            <Input
              {id}
              bind:value={scopeId}
              placeholder="Rule id"
              required
              mono
            />
          {/if}
        {/snippet}
      </Field>
    {/if}
    <div class="row">
      <Field label="Duration">
        {#snippet children({ id })}
          <Select {id} bind:value={preset}>
            {#each silencePresets as option (option.value)}
              <option value={option.value}>{option.label}</option>
            {/each}
          </Select>
        {/snippet}
      </Field>
      {#if preset === 'custom'}
        <Field label="Ends at" required>
          {#snippet children({ id })}
            <Input
              {id}
              type="datetime-local"
              bind:value={customEnd}
              required
              mono
            />
          {/snippet}
        </Field>
      {/if}
    </div>
    <Field
      label="Reason"
      required
      hint="Shown to anyone reviewing why notifications were paused."
    >
      {#snippet children({ id, describedBy })}
        <Input
          {id}
          bind:value={reason}
          required
          maxlength={500}
          placeholder="Planned maintenance"
          aria-describedby={describedBy}
          data-autofocus
        />
      {/snippet}
    </Field>
    {#if error}<p class="error" role="alert">{error}</p>{/if}
  </form>
  {#snippet footer()}
    <Button variant="ghost" onclick={() => (open = false)} disabled={busy}
      >Cancel</Button
    >
    <Button variant="primary" onclick={submit} loading={busy}
      >Create silence</Button
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
  }
  .error {
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
</style>
