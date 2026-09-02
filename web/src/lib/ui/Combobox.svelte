<script lang="ts" module>
  export interface ComboboxOption {
    value: string;
    label: string;
    hint?: string;
    /** Extra searchable words. */
    keywords?: string[];
  }
</script>

<script lang="ts">
  import { onDestroy, tick } from 'svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import { attachFloating } from './floating';
  import { clickOutside, uniqueId } from './focus';
  import { rankItems } from './palette';

  let {
    options,
    value = $bindable(''),
    placeholder = 'Search…',
    id = uniqueId('combobox'),
    disabled = false,
    invalid = false,
    required = false,
    label,
    onchange,
  }: {
    options: ComboboxOption[];
    /** Accessible name when no visible label references the input. */
    label?: string;
    value?: string;
    placeholder?: string;
    id?: string;
    disabled?: boolean;
    invalid?: boolean;
    required?: boolean;
    onchange?: (value: string) => void;
  } = $props();

  let open = $state(false);
  let query = $state('');
  let activeIndex = $state(0);
  let input = $state<HTMLInputElement | null>(null);
  let list = $state<HTMLUListElement | null>(null);
  let cleanup: (() => void) | null = null;
  const listId = $derived(`${id}-listbox`);

  const selected = $derived(
    options.find((option) => option.value === value) ?? null,
  );
  const results = $derived(
    rankItems(
      options.map((option) => ({
        id: option.value,
        group: '',
        label: option.label,
        hint: option.hint,
        keywords: option.keywords,
      })),
      query,
      50,
    ).map((item) => options.find((option) => option.value === item.id)!),
  );

  async function show() {
    if (disabled) return;
    open = true;
    activeIndex = Math.max(
      0,
      results.findIndex((option) => option.value === value),
    );
    await tick();
    if (input && list)
      cleanup = attachFloating(input, list, {
        placement: 'bottom-start',
        gap: 4,
        matchWidth: true,
      });
  }

  function hide() {
    open = false;
    cleanup?.();
    cleanup = null;
    query = '';
  }

  function choose(option: ComboboxOption) {
    value = option.value;
    onchange?.(option.value);
    hide();
  }

  function onKeydown(event: KeyboardEvent) {
    if (!open && (event.key === 'ArrowDown' || event.key === 'Enter')) {
      event.preventDefault();
      void show();
      return;
    }
    if (!open) return;
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      activeIndex = Math.min(results.length - 1, activeIndex + 1);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      activeIndex = Math.max(0, activeIndex - 1);
    } else if (event.key === 'Enter') {
      event.preventDefault();
      if (results[activeIndex]) choose(results[activeIndex]);
    } else if (event.key === 'Escape') {
      event.preventDefault();
      hide();
    } else if (event.key === 'Tab') {
      hide();
    }
  }

  $effect(() => {
    void query;
    activeIndex = 0;
  });

  $effect(() => {
    if (!open || !list) return;
    list
      .querySelector(`[data-index="${activeIndex}"]`)
      ?.scrollIntoView({ block: 'nearest' });
  });

  onDestroy(() => cleanup?.());
</script>

<div class="combobox" class:open class:invalid use:clickOutside={hide}>
  <input
    bind:this={input}
    {id}
    type="text"
    role="combobox"
    aria-expanded={open}
    aria-controls={listId}
    aria-autocomplete="list"
    aria-activedescendant={open && results[activeIndex]
      ? `${listId}-${activeIndex}`
      : undefined}
    aria-invalid={invalid ? 'true' : undefined}
    aria-required={required ? 'true' : undefined}
    aria-label={label}
    autocomplete="off"
    spellcheck="false"
    {disabled}
    placeholder={selected ? selected.label : placeholder}
    value={open ? query : (selected?.label ?? '')}
    oninput={(event) => {
      query = event.currentTarget.value;
      if (!open) void show();
    }}
    onfocus={() => void show()}
    onclick={() => void show()}
    onkeydown={onKeydown}
  />
  <span class="chevron" aria-hidden="true"><ChevronDown /></span>
  {#if open}
    <ul bind:this={list} id={listId} class="listbox" role="listbox">
      {#if !results.length}
        <li class="none">No matches</li>
      {/if}
      {#each results as option, index (option.value)}
        <li
          id={`${listId}-${index}`}
          data-index={index}
          role="option"
          aria-selected={option.value === value}
          class:active={index === activeIndex}
          onmousedown={(event) => event.preventDefault()}
          onmousemove={() => (activeIndex = index)}
          onmouseup={() => choose(option)}
        >
          <span class="label">{option.label}</span>
          {#if option.hint}<span class="hint">{option.hint}</span>{/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .combobox {
    position: relative;
    width: 100%;
  }
  input {
    width: 100%;
    height: var(--control-h);
    padding: 0 32px 0 var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    color: var(--text);
    font-size: var(--text-sm);
  }
  input::placeholder {
    color: var(--text-3);
  }
  .open input::placeholder {
    color: var(--text-2);
  }
  input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-bg);
  }
  .invalid input {
    border-color: var(--critical-solid);
  }
  input:disabled {
    opacity: 0.6;
  }
  .chevron {
    position: absolute;
    top: 0;
    bottom: 0;
    right: 10px;
    display: inline-flex;
    align-items: center;
    color: var(--text-3);
    pointer-events: none;
  }
  .chevron :global(svg) {
    width: 14px;
    height: 14px;
  }
  .listbox {
    z-index: 70;
    max-height: 260px;
    margin: 0;
    padding: var(--space-1);
    overflow-y: auto;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    background: var(--surface);
    box-shadow: var(--shadow-lg);
    list-style: none;
  }
  [role='option'] {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-height: 34px;
    padding: 0 var(--space-2);
    border-radius: var(--radius-sm);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  [role='option'].active {
    background: var(--surface-2);
  }
  [role='option'][aria-selected='true'] .label {
    color: var(--accent-text);
  }
  .label {
    font-weight: 500;
  }
  .hint {
    flex: 1;
    min-width: 0;
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .none {
    padding: var(--space-3);
    color: var(--text-3);
    font-size: var(--text-sm);
  }
</style>
