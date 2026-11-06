<script lang="ts">
  let {
    steps,
    current,
    label,
  }: {
    steps: string[];
    /** One-based index of the step being shown. */
    current: number;
    label: string;
  } = $props();
</script>

<ol class="steps" aria-label={label}>
  {#each steps as step, index (step)}
    {@const position = index + 1}
    <li
      class:done={position < current}
      class:current={position === current}
      aria-current={position === current ? 'step' : undefined}
    >
      <span class="number" aria-hidden="true">{position}</span>
      <span class="name">{step}</span>
      {#if position < current}<span class="sr-only">(done)</span>{/if}
    </li>
  {/each}
</ol>

<style>
  .steps {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-4);
    margin: 0;
    padding: 0 0 var(--space-4);
    border-bottom: 1px solid var(--line);
    list-style: none;
  }
  li {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--muted);
    font-size: var(--text-meta);
    font-weight: var(--weight-medium);
  }
  .number {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-control);
    font-family: var(--font-mono);
    font-size: var(--text-label);
    line-height: 1;
  }
  li.current {
    color: var(--fg);
  }
  li.current .number {
    border-color: var(--accent);
    background: var(--accent);
    color: var(--accent-contrast);
  }
  li.done .number {
    border-color: var(--accent-border);
    color: var(--accent-text);
  }
</style>
