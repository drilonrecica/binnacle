<script lang="ts">
  import type { Snippet } from 'svelte';
  import Button from '../ui/Button.svelte';
  import ErrorState from '../ui/ErrorState.svelte';
  import StaleBanner from '../ui/StaleBanner.svelte';
  import type { TabSource } from './tab-source.svelte';

  /**
   * Wraps one tab's content in its own load state (UI-9). A source that never
   * loaded shows an error with a retry instead of an empty table; one whose
   * latest refresh failed keeps its rows under a stale notice.
   */
  let {
    source,
    what,
    onretry,
    children,
  }: {
    source: TabSource<unknown>;
    /** Plural noun for messages, e.g. "Open incidents". */
    what: string;
    onretry: () => void;
    children: Snippet;
  } = $props();
</script>

{#if source.failed}
  <div class="pad">
    <ErrorState
      title={`${what} could not be loaded`}
      description={source.error}
      {onretry}
    />
  </div>
{:else}
  {#if source.stale}
    <div class="pad">
      <StaleBanner
        message={`${what} could not be refreshed: ${source.error}`}
        since={source.loadedAt}
      >
        <Button size="sm" variant="quiet" onclick={onretry}>Try again</Button>
      </StaleBanner>
    </div>
  {/if}
  {@render children()}
{/if}

<style>
  .pad {
    padding: var(--space-3) var(--space-4);
  }
</style>
