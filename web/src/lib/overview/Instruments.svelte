<script lang="ts">
  import HardDrive from '@lucide/svelte/icons/hard-drive';
  import Gauge from '../ui/Gauge.svelte';
  import { instrumentName, type InstrumentSet } from './headroom';

  let { set, stale = false }: { set: InstrumentSet; stale?: boolean } =
    $props();
</script>

<section
  class="instruments"
  class:stale
  aria-labelledby="headroom-title"
  data-testid="instruments"
>
  <h2 id="headroom-title" class="sr-only">Host headroom</h2>
  <ul>
    {#each set.instruments as instrument (instrument.key)}
      <li>
        <a
          class="instrument"
          href={instrument.href}
          aria-label={instrumentName(instrument)}
        >
          <Gauge
            value={instrument.value}
            label={instrument.label}
            sublabel={instrument.path}
            valueText={instrument.valueText}
            unit={instrument.unit}
            caption={instrument.caption}
            warn={instrument.thresholds.warn}
            threshold={instrument.thresholds.critical}
            trend={instrument.trend.text}
            trendTone={instrument.trend.tone}
          />
        </a>
      </li>
    {/each}
    {#if set.moreDisks > 0}
      <li>
        <a class="instrument more" href="/server?tab=filesystems">
          <HardDrive aria-hidden="true" />
          <span class="more-count">{set.moreDisks} more disks</span>
          <span class="more-hint">on Server › Filesystems</span>
        </a>
      </li>
    {/if}
  </ul>
</section>

<style>
  .instruments {
    margin-bottom: var(--space-4);
    border: 1px solid var(--line);
    border-radius: var(--radius-card);
    background: var(--panel);
    transition: opacity var(--motion) var(--ease-out);
  }
  /* Old numbers stay readable but recede while the stream is stale (UI-1). */
  .stale {
    opacity: 0.55;
  }
  ul {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(168px, 1fr));
    margin: 0;
    padding: 0;
    list-style: none;
  }
  li {
    display: flex;
    min-width: 0;
  }
  .instrument {
    display: grid;
    flex: 1;
    align-content: start;
    min-width: 0;
    padding: var(--space-4) var(--space-3);
    border-radius: var(--radius-card);
    color: var(--fg);
    text-decoration: none;
  }
  .instrument:hover {
    background: var(--panel-2);
    text-decoration: none;
  }
  .more {
    place-content: center;
    justify-items: center;
    gap: var(--space-1);
    color: var(--muted);
    text-align: center;
  }
  .more :global(svg) {
    width: 20px;
    height: 20px;
  }
  .more-count {
    color: var(--fg);
    font-weight: var(--weight-semibold);
  }
  .more-hint {
    font-size: var(--text-meta);
  }
  @media (max-width: 560px) {
    ul {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
