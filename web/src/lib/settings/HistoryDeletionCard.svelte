<script lang="ts">
  import { onDestroy } from 'svelte';
  import type { LiveSnapshot } from '../live.svelte';
  import {
    controlDeletion,
    deletionJob,
    previewDeletion,
    startDeletion,
    type DeletionJob,
    type DeletionKind,
    type DeletionPreview,
  } from '../api/access';
  import { errorMessage } from '../api/client';
  import ResourcePicker from '../resources/ResourcePicker.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import ConfirmDialog from '../ui/ConfirmDialog.svelte';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import ProgressBar from '../ui/ProgressBar.svelte';
  import RadioCards from '../ui/RadioCards.svelte';
  import { toasts } from '../ui/toast.svelte';

  let {
    snapshot = null,
    archivedResourceId = '',
  }: {
    snapshot?: LiveSnapshot | null;
    /** Locks the card to one archived resource (used on its detail page). */
    archivedResourceId?: string;
  } = $props();

  const locked = $derived(Boolean(archivedResourceId));
  let kind = $state<DeletionKind>('before');
  let resourceId = $state('');
  $effect(() => {
    if (archivedResourceId) {
      kind = 'archived_resource';
      resourceId = archivedResourceId;
    }
  });
  let before = $state('');
  let preview = $state<DeletionPreview | null>(null);
  let confirmOpen = $state(false);
  let busy = $state(false);
  let error = $state('');
  let job = $state<DeletionJob | null>(null);
  let timer: number | undefined;

  const options = $derived<
    Array<{
      value: DeletionKind;
      title: string;
      description: string;
    }>
  >(
    locked
      ? [
          {
            value: 'archived_resource',
            title: 'This archived resource',
            description: 'Remove every sample and event kept for it.',
          },
        ]
      : [
          {
            value: 'before',
            title: 'Older than a date',
            description:
              'Trim history for the whole server before a point in time.',
          },
          {
            value: 'resource',
            title: 'One resource',
            description: 'Remove a single resource’s samples and events.',
          },
          {
            value: 'archived_resource',
            title: 'Archived resource',
            description:
              'Remove history for a resource that is no longer running.',
          },
          {
            value: 'all',
            title: 'Everything',
            description:
              'Reset all monitoring history. Settings and accounts stay.',
          },
        ],
  );

  async function requestPreview() {
    error = '';
    preview = null;
    if ((kind === 'resource' || kind === 'archived_resource') && !resourceId) {
      error = 'Choose a resource.';
      return;
    }
    if (kind === 'before' && !before) {
      error = 'Choose a cutoff date.';
      return;
    }
    busy = true;
    try {
      preview = await previewDeletion({
        kind,
        before: kind === 'before' ? new Date(before).toISOString() : undefined,
        resourceId:
          kind === 'resource' || kind === 'archived_resource'
            ? resourceId
            : undefined,
      });
      confirmOpen = true;
    } catch (reason) {
      error = errorMessage(reason);
    } finally {
      busy = false;
    }
  }

  async function start() {
    if (!preview) return;
    job = await startDeletion(preview.token, preview.confirmation);
    preview = null;
    toasts.success('History deletion started');
    poll();
  }

  function poll() {
    window.clearTimeout(timer);
    if (!job) return;
    timer = window.setTimeout(async () => {
      if (!job) return;
      try {
        job = await deletionJob(job.id);
      } catch {
        /* Keep the last known state. */
      }
      if (job && ['queued', 'running', 'cancelling'].includes(job.state))
        poll();
    }, 1000);
  }

  async function control(action: 'cancel' | 'retry') {
    if (!job) return;
    try {
      await controlDeletion(job.id, action);
      job = await deletionJob(job.id);
      poll();
    } catch (reason) {
      toasts.error('Could not update the job', {
        description: errorMessage(reason),
      });
    }
  }

  onDestroy(() => window.clearTimeout(timer));

  const progress = $derived(
    job && job.totalRows > 0 ? (job.deletedRows / job.totalRows) * 100 : null,
  );
</script>

<Card
  title="Delete history"
  description="Deletion is permanent. Preview the row count, confirm by typing the phrase, then watch the job run."
  tone="danger"
>
  <div class="stack">
    {#if !archivedResourceId}
      <RadioCards
        label="What to delete"
        bind:value={kind}
        {options}
        columns={2}
        onchange={() => (preview = null)}
      />
    {/if}
    {#if kind === 'before'}
      <Field
        label="Delete everything before"
        hint="Samples, rollups, and events older than this moment."
      >
        {#snippet children({ id, describedBy })}
          <Input
            {id}
            type="datetime-local"
            bind:value={before}
            mono
            aria-describedby={describedBy}
          />
        {/snippet}
      </Field>
    {:else if (kind === 'resource' || kind === 'archived_resource') && !archivedResourceId}
      <Field
        label="Resource"
        hint={kind === 'archived_resource'
          ? 'Enter the archived resource id from the Archived tab.'
          : undefined}
      >
        {#snippet children({ id, describedBy })}
          {#if kind === 'resource'}
            <ResourcePicker {id} {snapshot} bind:value={resourceId} />
          {:else}
            <Input
              {id}
              bind:value={resourceId}
              mono
              placeholder="res_…"
              aria-describedby={describedBy}
            />
          {/if}
        {/snippet}
      </Field>
    {/if}
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <div class="actions">
      <Button variant="danger" onclick={requestPreview} loading={busy}
        >Preview deletion…</Button
      >
    </div>
    {#if job}
      <div class="job" role="status" aria-live="polite">
        <div class="job-head">
          <strong
            >{job.state === 'completed'
              ? 'Deletion complete'
              : job.state === 'failed'
                ? 'Deletion failed'
                : job.state === 'cancelled'
                  ? 'Deletion cancelled'
                  : `Deleting… (${job.state})`}</strong
          >
          <span class="num"
            >{job.deletedRows.toLocaleString()} of {job.totalRows.toLocaleString()}
            rows</span
          >
        </div>
        <ProgressBar
          value={progress ?? (job.state === 'completed' ? 100 : 0)}
          label="Deletion progress"
          tone={job.state === 'failed' ? 'critical' : 'accent'}
        />
        {#if job.error}<p class="error">{job.error}</p>{/if}
        <div class="actions">
          {#if ['queued', 'running'].includes(job.state)}
            <Button size="sm" onclick={() => control('cancel')}>Cancel</Button>
          {:else if job.state === 'failed'}
            <Button size="sm" onclick={() => control('retry')}>Retry</Button>
          {/if}
        </div>
      </div>
    {/if}
  </div>
</Card>

<ConfirmDialog
  bind:open={confirmOpen}
  title="Delete history permanently?"
  description={preview
    ? `${preview.totalRows.toLocaleString()} rows will be removed. This cannot be undone.`
    : ''}
  confirmLabel="Delete history"
  phrase={preview?.confirmation}
  onconfirm={start}
/>

<style>
  .stack {
    display: grid;
    gap: var(--space-4);
  }
  .actions {
    display: flex;
    gap: var(--space-2);
  }
  .error {
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  .job {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    font-size: var(--text-sm);
  }
  .job-head {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
  }
</style>
