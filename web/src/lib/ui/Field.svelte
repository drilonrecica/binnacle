<script lang="ts">
  import type { Snippet } from 'svelte';
  import { uniqueId } from './focus';

  let {
    label,
    hint,
    error,
    required = false,
    id = uniqueId('field'),
    inline = false,
    children,
  }: {
    label: string;
    hint?: string;
    error?: string;
    required?: boolean;
    id?: string;
    /** Label beside the control (for switches and checkboxes). */
    inline?: boolean;
    children: Snippet<
      [{ id: string; describedBy: string | undefined; invalid: boolean }]
    >;
  } = $props();

  const hintId = $derived(hint ? `${id}-hint` : undefined);
  const errorId = $derived(error ? `${id}-error` : undefined);
  const describedBy = $derived(
    [errorId, hintId].filter(Boolean).join(' ') || undefined,
  );
</script>

<div class="field" class:inline class:invalid={Boolean(error)}>
  <label for={id} class="label">
    {label}{#if required}<span class="required" aria-hidden="true">*</span>{/if}
  </label>
  <div class="control">
    {@render children({ id, describedBy, invalid: Boolean(error) })}
  </div>
  {#if hint && !error}<p id={hintId} class="hint">{hint}</p>{/if}
  {#if error}<p id={errorId} class="error" role="alert">{error}</p>{/if}
</div>

<style>
  .field {
    display: grid;
    gap: var(--space-2);
    min-width: 0;
  }
  .inline {
    grid-template-columns: auto 1fr;
    grid-template-areas:
      'control label'
      'control hint';
    align-items: center;
    column-gap: var(--space-3);
    row-gap: 2px;
  }
  .inline .label {
    grid-area: label;
  }
  .inline .control {
    grid-area: control;
  }
  .inline .hint,
  .inline .error {
    grid-area: hint;
  }
  .label {
    font-size: var(--text-sm);
    font-weight: 500;
  }
  .required {
    margin-left: 2px;
    color: var(--critical-fg);
  }
  .hint {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .error {
    color: var(--critical-fg);
    font-size: var(--text-xs);
  }
</style>
