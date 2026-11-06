<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { mounts } from './mounts';

  let { id = '' }: { id?: string } = $props();
  const first = untrack(() => id);
  onMount(() => {
    mounts.push(first);
  });
</script>

<p data-testid="stub">mounted for {first}, showing {id}</p>
