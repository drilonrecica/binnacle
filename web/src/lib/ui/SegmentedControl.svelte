<script lang="ts" generics="V extends string">
  let {
    options,
    value = $bindable(),
    label,
    size = 'md',
    onchange,
  }: {
    options: Array<{ value: V; label: string; disabled?: boolean }>;
    value?: V;
    label: string;
    size?: 'sm' | 'md';
    onchange?: (value: V) => void;
  } = $props();

  function select(next: V) {
    if (next === value) return;
    value = next;
    onchange?.(next);
  }
</script>

<div class={`segmented ${size}`} role="group" aria-label={label}>
  {#each options as option (option.value)}
    <button
      type="button"
      aria-pressed={value === option.value}
      class:active={value === option.value}
      disabled={option.disabled}
      onclick={() => select(option.value)}
    >
      {option.label}
    </button>
  {/each}
</div>

<style>
  .segmented {
    display: inline-flex;
    padding: 2px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
  }
  button {
    height: calc(var(--control-h) - 6px);
    padding: 0 var(--space-3);
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--text-2);
    font-size: var(--text-sm);
    font-weight: 500;
    font-family: var(--font-mono);
    transition:
      background var(--motion-fast) var(--ease),
      color var(--motion-fast) var(--ease);
  }
  .sm button {
    height: calc(var(--control-h-sm) - 6px);
    padding: 0 var(--space-2);
    font-size: var(--text-xs);
  }
  button:hover:not(:disabled) {
    color: var(--text);
  }
  button.active {
    background: var(--surface);
    color: var(--text);
    box-shadow: var(--shadow-sm);
  }
  button:disabled {
    opacity: 0.5;
  }
</style>
