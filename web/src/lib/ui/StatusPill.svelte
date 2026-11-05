<script lang="ts">
  import Status from './Status.svelte';
  import type { NormalizedStatus } from './status';

  // Kept for existing callers: a Status derived from a raw API status string.
  let {
    status,
    label,
    size = 'md',
  }: {
    status: string | NormalizedStatus | null | undefined;
    label?: string;
    size?: 'sm' | 'md';
  } = $props();
</script>

<Status {status} {label} {size} />
