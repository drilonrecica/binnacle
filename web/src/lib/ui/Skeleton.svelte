<script lang="ts">
  let {
    width = '100%',
    height = 16,
    radius = 'var(--radius-sm)',
    lines = 1,
  }: {
    width?: string | number;
    height?: number;
    radius?: string;
    lines?: number;
  } = $props();
  const w = $derived(typeof width === 'number' ? `${width}px` : width);
</script>

{#each Array.from({ length: lines }, (_, index) => index) as index (index)}
  <span
    class="skeleton"
    style:width={index === lines - 1 && lines > 1 ? '70%' : w}
    style:height={`${height}px`}
    style:border-radius={radius}
    aria-hidden="true"
  ></span>
{/each}

<style>
  .skeleton {
    display: block;
    background: linear-gradient(
      90deg,
      var(--surface-2) 25%,
      var(--surface-3) 50%,
      var(--surface-2) 75%
    );
    background-size: 200% 100%;
    animation: shimmer 1.4s ease-in-out infinite;
  }
  .skeleton + .skeleton {
    margin-top: var(--space-2);
  }
  @keyframes shimmer {
    from {
      background-position: 200% 0;
    }
    to {
      background-position: -200% 0;
    }
  }
</style>
