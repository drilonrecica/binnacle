<script lang="ts">
  import Menu from './Menu.svelte';
  import MenuItem from './MenuItem.svelte';

  let { onselect }: { onselect: (id: string) => void } = $props();
</script>

<Menu label="Resource actions">
  {#snippet trigger(props)}
    <button type="button" {...props}>Actions</button>
  {/snippet}
  <MenuItem onselect={() => onselect('pin')}>Pin</MenuItem>
  <MenuItem onselect={() => onselect('silence')}>Silence</MenuItem>
</Menu>
