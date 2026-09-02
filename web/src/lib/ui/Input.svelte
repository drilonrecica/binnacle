<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLInputAttributes } from 'svelte/elements';

  let {
    value = $bindable(''),
    invalid = false,
    mono = false,
    leading,
    trailing,
    class: className = '',
    ...rest
  }: {
    value?: string;
    invalid?: boolean;
    mono?: boolean;
    leading?: Snippet;
    trailing?: Snippet;
    class?: string;
  } & HTMLInputAttributes = $props();
</script>

<div
  class={`input-wrap ${className}`}
  class:invalid
  class:has-leading={Boolean(leading)}
  class:has-trailing={Boolean(trailing)}
>
  {#if leading}<span class="adornment leading">{@render leading()}</span>{/if}
  <input
    {...rest}
    bind:value
    class:mono
    aria-invalid={invalid ? 'true' : undefined}
  />
  {#if trailing}<span class="adornment trailing">{@render trailing()}</span
    >{/if}
</div>

<style>
  .input-wrap {
    position: relative;
    display: flex;
    align-items: center;
    width: 100%;
  }
  input {
    width: 100%;
    height: var(--control-h);
    padding: 0 var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    color: var(--text);
    font-size: var(--text-sm);
    transition:
      border-color var(--motion-fast) var(--ease),
      box-shadow var(--motion-fast) var(--ease);
  }
  input.mono {
    font-family: var(--font-mono);
  }
  input::placeholder {
    color: var(--text-3);
  }
  input:hover:not(:disabled) {
    border-color: var(--n-600);
  }
  input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-bg);
  }
  input:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  .invalid input {
    border-color: var(--critical-solid);
  }
  .invalid input:focus {
    box-shadow: 0 0 0 3px var(--critical-bg);
  }
  .has-leading input {
    padding-left: 32px;
  }
  .has-trailing input {
    padding-right: 32px;
  }
  .adornment {
    position: absolute;
    top: 0;
    bottom: 0;
    display: inline-flex;
    align-items: center;
    color: var(--text-3);
    pointer-events: none;
  }
  .adornment :global(svg) {
    width: 15px;
    height: 15px;
  }
  .leading {
    left: 10px;
  }
  .trailing {
    right: 10px;
  }
</style>
