<script lang="ts">
  import Button from './Button.svelte';
  import Dialog from './Dialog.svelte';

  let open = $state(false);
</script>

<Button onclick={() => (open = true)}>Open</Button>
<Dialog bind:open title="Delete history">
  <p>History older than 30 days is removed.</p>
</Dialog>
