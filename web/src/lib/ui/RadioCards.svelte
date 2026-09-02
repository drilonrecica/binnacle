<script lang="ts" generics="V extends string">
  import type { Snippet } from 'svelte';
  import { uniqueId } from './focus';

  let {
    options,
    value = $bindable(),
    name = uniqueId('radio'),
    label,
    columns = 1,
    disabled = false,
    extra,
    onchange,
  }: {
    options: Array<{
      value: V;
      title: string;
      description?: string;
      meta?: string;
    }>;
    value?: V;
    name?: string;
    label: string;
    columns?: 1 | 2 | 3;
    disabled?: boolean;
    /** Extra content rendered inside each card, e.g. a tier table. */
    extra?: Snippet<[V]>;
    onchange?: (value: V) => void;
  } = $props();
</script>

<fieldset class={`cards cols-${columns}`} {disabled}>
  <legend class="sr-only">{label}</legend>
  {#each options as option (option.value)}
    <label class="card" class:selected={value === option.value}>
      <input
        type="radio"
        {name}
        value={option.value}
        checked={value === option.value}
        onchange={() => {
          value = option.value;
          onchange?.(option.value);
        }}
      />
      <span class="indicator" aria-hidden="true"></span>
      <span class="text">
        <span class="title">{option.title}</span>
        {#if option.description}<span class="description"
            >{option.description}</span
          >{/if}
        {#if option.meta}<span class="meta">{option.meta}</span>{/if}
        {#if extra}<span class="extra">{@render extra(option.value)}</span>{/if}
      </span>
    </label>
  {/each}
</fieldset>

<style>
  .cards {
    display: grid;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    border: 0;
  }
  .cols-2 {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .cols-3 {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  .card {
    position: relative;
    display: flex;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    background: var(--surface);
    cursor: pointer;
    transition:
      border-color var(--motion-fast) var(--ease),
      background var(--motion-fast) var(--ease);
  }
  .card:hover {
    border-color: var(--n-600);
  }
  .card.selected {
    border-color: var(--accent);
    background: var(--accent-bg);
  }
  .card:has(input:focus-visible) {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }
  input {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    margin: 0;
    opacity: 0;
    cursor: pointer;
  }
  .indicator {
    flex: none;
    width: 16px;
    height: 16px;
    margin-top: 2px;
    border: 2px solid var(--border-strong);
    border-radius: 50%;
    background: var(--bg-subtle);
  }
  .selected .indicator {
    border-color: var(--accent);
    background: radial-gradient(circle, var(--accent) 45%, transparent 50%);
  }
  .text {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .title {
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .description {
    color: var(--text-2);
    font-size: var(--text-sm);
  }
  .meta {
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .extra {
    margin-top: var(--space-2);
  }
  fieldset:disabled .card {
    opacity: 0.6;
    cursor: not-allowed;
  }
  @media (max-width: 720px) {
    .cols-2,
    .cols-3 {
      grid-template-columns: 1fr;
    }
  }
</style>
