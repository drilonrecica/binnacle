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
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-control);
    background: var(--bg);
  }
  button {
    height: calc(var(--control-h) - 6px);
    padding: 0 var(--space-3);
    border: 0;
    border-radius: 2px;
    background: transparent;
    color: var(--muted);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    font-weight: var(--weight-medium);
    transition:
      background var(--motion) var(--ease-out),
      color var(--motion) var(--ease-out);
  }
  .sm button {
    height: calc(var(--control-h-sm) - 6px);
    padding: 0 var(--space-2);
  }
  button:hover:not(:disabled) {
    color: var(--fg);
  }
  button.active {
    background: var(--panel-2);
    color: var(--fg);
    box-shadow: inset 0 -2px 0 var(--accent);
  }
  button:disabled {
    opacity: 0.5;
  }
</style>
