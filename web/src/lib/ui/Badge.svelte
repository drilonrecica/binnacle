<script lang="ts">
  import type { Snippet } from 'svelte';
  import StatusMarker from './StatusMarker.svelte';
  import type { Tone } from './status';

  let {
    tone = 'neutral',
    dot = false,
    mono = false,
    children,
  }: {
    tone?: Tone;
    /** Shows the tone's shape marker before the text. */
    dot?: boolean;
    mono?: boolean;
    children: Snippet;
  } = $props();
</script>

<span class={`badge ${tone}`} class:mono data-tone={tone}>
  {#if dot}<StatusMarker {tone} size={7} />{/if}
  {@render children()}
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    height: 22px;
    padding: 0 var(--space-2);
    border: 1px solid var(--badge-border);
    border-radius: var(--radius-control);
    background: var(--badge-bg);
    color: var(--badge-fg);
    font-size: var(--text-meta);
    font-weight: var(--weight-medium);
    line-height: 1;
    white-space: nowrap;
  }
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    font-variant-numeric: tabular-nums;
  }
  .ok {
    --badge-bg: var(--ok-bg);
    --badge-fg: var(--ok-fg);
    --badge-border: var(--ok-border);
  }
  .warn {
    --badge-bg: var(--warn-bg);
    --badge-fg: var(--warn-fg);
    --badge-border: var(--warn-border);
  }
  .critical {
    --badge-bg: var(--critical-bg);
    --badge-fg: var(--critical-fg);
    --badge-border: var(--critical-border);
  }
  .info {
    --badge-bg: var(--info-bg);
    --badge-fg: var(--info-fg);
    --badge-border: var(--info-border);
  }
  .neutral {
    --badge-bg: var(--neutral-bg);
    --badge-fg: var(--neutral-fg);
    --badge-border: var(--neutral-border);
  }
  .accent {
    --badge-bg: var(--accent-bg);
    --badge-fg: var(--accent-text);
    --badge-border: var(--accent-border);
  }
</style>
