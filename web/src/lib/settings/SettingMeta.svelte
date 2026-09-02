<script lang="ts">
  import type { SettingValue } from '../api/settings';
  import { sourceLabel } from '../api/settings';
  import Badge from '../ui/Badge.svelte';

  let { setting }: { setting: SettingValue | undefined } = $props();
</script>

{#if setting}
  <span class="meta">
    <Badge tone="neutral">{sourceLabel(setting.source)}</Badge>
    {#if setting.applyMode === 'restart_required'}<Badge tone="warn"
        >Restart required</Badge
      >{:else}<span class="live">applies live</span>{/if}
  </span>
{/if}

<style>
  .meta {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }
  .live {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
</style>
