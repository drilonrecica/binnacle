<script lang="ts">
  let { size = 16, label = '' }: { size?: number; label?: string } = $props();
</script>

<svg
  class="spinner"
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  role={label ? 'img' : undefined}
  aria-label={label || undefined}
  aria-hidden={label ? undefined : 'true'}
>
  <circle
    cx="12"
    cy="12"
    r="9"
    stroke="currentColor"
    stroke-opacity="0.25"
    stroke-width="2.5"
  />
  <path
    d="M21 12a9 9 0 0 0-9-9"
    stroke="currentColor"
    stroke-width="2.5"
    stroke-linecap="round"
  />
</svg>

<style>
  .spinner {
    display: inline-block;
    flex: none;
    animation: spin 0.9s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
