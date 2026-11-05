<script lang="ts">
  import { toneShape, type Shape, type Tone } from './status';

  let {
    tone = 'neutral',
    shape,
    size = 8,
  }: {
    tone?: Tone;
    /** Overrides the tone's own shape; rarely needed. */
    shape?: Shape;
    size?: number;
  } = $props();

  const resolved = $derived(shape ?? toneShape(tone));
</script>

<!-- Decorative: the adjacent word or the parent's accessible name carries the meaning. -->
<svg
  class={`marker ${tone}`}
  data-shape={resolved}
  width={size}
  height={size}
  viewBox="0 0 10 10"
  aria-hidden="true"
  focusable="false"
>
  {#if resolved === 'circle'}
    <circle cx="5" cy="5" r="4.25" class="fill" />
  {:else if resolved === 'ring'}
    <circle cx="5" cy="5" r="3.5" class="stroke" />
  {:else if resolved === 'square'}
    <rect x="1" y="1" width="8" height="8" rx="1" class="fill" />
  {:else if resolved === 'diamond'}
    <path d="M5 1 L9 5 L5 9 L1 5 Z" class="stroke" />
  {:else}
    <circle cx="5" cy="5" r="3.75" class="stroke dashed" />
  {/if}
</svg>

<style>
  .marker {
    display: inline-block;
    flex: none;
    overflow: visible;
    color: var(--marker);
  }
  .fill {
    fill: currentColor;
  }
  .stroke {
    fill: none;
    stroke: currentColor;
    stroke-width: 1.75;
  }
  .dashed {
    stroke-width: 1.5;
    stroke-dasharray: 2.1 1.8;
  }
  .ok {
    --marker: var(--ok-solid);
  }
  .warn {
    --marker: var(--warn-solid);
  }
  .critical {
    --marker: var(--critical-solid);
  }
  .info {
    --marker: var(--info-solid);
  }
  .neutral {
    --marker: var(--neutral-solid);
  }
  .accent {
    --marker: var(--accent);
  }
</style>
