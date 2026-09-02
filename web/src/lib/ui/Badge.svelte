<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Tone } from './status';

  let {
    tone = 'neutral',
    dot = false,
    mono = false,
    children,
  }: {
    tone?: Tone;
    dot?: boolean;
    mono?: boolean;
    children: Snippet;
  } = $props();
</script>

<span class={`badge ${tone}`} class:mono>
  {#if dot}<span class="dot" aria-hidden="true"></span>{/if}
  {@render children()}
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    height: 22px;
    padding: 0 0.5rem;
    border: 1px solid var(--badge-border);
    border-radius: var(--radius-full);
    background: var(--badge-bg);
    color: var(--badge-fg);
    font-size: var(--text-xs);
    font-weight: 500;
    line-height: 1;
    white-space: nowrap;
  }
  .mono {
    font-family: var(--font-mono);
  }
  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--badge-solid);
  }
  .ok {
    --badge-bg: var(--ok-bg);
    --badge-fg: var(--ok-fg);
    --badge-border: var(--ok-border);
    --badge-solid: var(--ok-solid);
  }
  .warn {
    --badge-bg: var(--warn-bg);
    --badge-fg: var(--warn-fg);
    --badge-border: var(--warn-border);
    --badge-solid: var(--warn-solid);
  }
  .critical {
    --badge-bg: var(--critical-bg);
    --badge-fg: var(--critical-fg);
    --badge-border: var(--critical-border);
    --badge-solid: var(--critical-solid);
  }
  .info {
    --badge-bg: var(--info-bg);
    --badge-fg: var(--info-fg);
    --badge-border: var(--info-border);
    --badge-solid: var(--info-solid);
  }
  .neutral {
    --badge-bg: var(--neutral-bg);
    --badge-fg: var(--neutral-fg);
    --badge-border: var(--neutral-border);
    --badge-solid: var(--neutral-solid);
  }
  .accent {
    --badge-bg: var(--accent-bg);
    --badge-fg: var(--accent-text);
    --badge-border: var(--accent-border);
    --badge-solid: var(--accent);
  }
</style>
