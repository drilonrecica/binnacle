<script lang="ts">
  import type { LiveSnapshot } from '../live.svelte';
  import { resourceGroup } from '../api/resources';
  import Combobox from '../ui/Combobox.svelte';

  let {
    snapshot,
    value = $bindable(''),
    id,
    invalid = false,
    required = false,
    disabled = false,
    includeInfrastructure = true,
    label,
    onchange,
  }: {
    label?: string;
    snapshot: LiveSnapshot | null;
    value?: string;
    id?: string;
    invalid?: boolean;
    required?: boolean;
    disabled?: boolean;
    includeInfrastructure?: boolean;
    onchange?: (value: string) => void;
  } = $props();

  const options = $derived(
    (snapshot?.resources ?? [])
      .filter(
        (resource) =>
          resource.status !== 'archived' &&
          (includeInfrastructure || !resource.infrastructure),
      )
      .map((resource) => ({
        value: resource.id,
        label: resource.name,
        hint:
          resourceGroup(resource) === 'Ungrouped'
            ? (resource.context ?? resource.category ?? '')
            : resourceGroup(resource),
        keywords: [
          resource.id,
          resource.project ?? '',
          resource.environment ?? '',
        ],
      }))
      .sort((a, b) => a.label.localeCompare(b.label)),
  );
</script>

<Combobox
  {options}
  bind:value
  {id}
  {label}
  {invalid}
  {required}
  {disabled}
  placeholder="Search resources…"
  {onchange}
/>
