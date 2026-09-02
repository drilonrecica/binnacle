<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLSelectAttributes } from 'svelte/elements';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';

  let {
    value = $bindable(''),
    invalid = false,
    size = 'md',
    class: className = '',
    children,
    ...rest
  }: {
    value?: string;
    invalid?: boolean;
    size?: 'sm' | 'md';
    class?: string;
    children: Snippet;
  } & Omit<HTMLSelectAttributes, 'size'> = $props();
</script>

<div class={`select-wrap ${size} ${className}`} class:invalid>
  <select {...rest} bind:value aria-invalid={invalid ? 'true' : undefined}>
    {@render children()}
  </select>
  <span class="chevron" aria-hidden="true"><ChevronDown /></span>
</div>

<style>
  .select-wrap {
    position: relative;
    display: inline-flex;
    width: 100%;
  }
  select {
    width: 100%;
    height: var(--control-h);
    padding: 0 32px 0 var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    color: var(--text);
    font-size: var(--text-sm);
    appearance: none;
    cursor: pointer;
  }
  .sm select {
    height: var(--control-h-sm);
    padding-left: var(--space-2);
    font-size: var(--text-xs);
  }
  select:hover:not(:disabled) {
    border-color: var(--n-600);
  }
  select:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-bg);
  }
  select:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  .invalid select {
    border-color: var(--critical-solid);
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
</style>
