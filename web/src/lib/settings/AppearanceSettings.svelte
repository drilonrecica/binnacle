<script lang="ts">
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import Pin from '@lucide/svelte/icons/pin';
  import X from '@lucide/svelte/icons/x';
  import type { LiveSnapshot } from '../live.svelte';
  import { errorMessage } from '../api/client';
  import { prefs } from '../preferences.svelte';
  import {
    chartRanges,
    landingPages,
    type ChartRange,
    type Density,
    type LandingPage,
    type Theme,
  } from '../preferences';
  import { navigation } from '../shell/navigation';
  import ResourcePicker from '../resources/ResourcePicker.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import Field from '../ui/Field.svelte';
  import IconButton from '../ui/IconButton.svelte';
  import RadioCards from '../ui/RadioCards.svelte';
  import Select from '../ui/Select.svelte';
  import { toasts } from '../ui/toast.svelte';

  let { live = null }: { live?: LiveSnapshot | null } = $props();

  let pick = $state('');
  const names = $derived(
    new Map(
      (live?.resources ?? []).map((resource) => [resource.id, resource.name]),
    ),
  );

  async function save(patch: Parameters<typeof prefs.save>[0], label: string) {
    try {
      await prefs.save(patch);
      toasts.success(label);
    } catch (reason) {
      toasts.error('Preference could not be saved', {
        description: errorMessage(reason),
      });
    }
  }

  function movePin(index: number, delta: number) {
    const pins = [...prefs.value.pinnedResources];
    const target = index + delta;
    if (target < 0 || target >= pins.length) return;
    [pins[index], pins[target]] = [pins[target], pins[index]];
    void save({ pinnedResources: pins }, 'Pins reordered');
  }

  function addPin() {
    if (!pick || prefs.value.pinnedResources.includes(pick)) return;
    if (prefs.value.pinnedResources.length >= 12) {
      toasts.warning('Up to 12 pins', {
        description: 'Remove a pinned resource before adding another.',
      });
      return;
    }
    void save(
      { pinnedResources: [...prefs.value.pinnedResources, pick] },
      'Resource pinned',
    );
    pick = '';
  }
</script>

<div class="stack">
  <Card title="Theme" description="Applies to this account on every browser.">
    <RadioCards
      label="Theme"
      columns={3}
      value={prefs.value.theme}
      options={[
        {
          value: 'system' as Theme,
          title: 'System',
          description: 'Follow the operating system.',
        },
        {
          value: 'dark' as Theme,
          title: 'Dark',
          description: 'Deep navy with teal accents.',
        },
        {
          value: 'light' as Theme,
          title: 'Light',
          description: 'Bright surfaces, same structure.',
        },
      ]}
      onchange={(theme) => void save({ theme }, 'Theme saved')}
    />
  </Card>

  <Card title="Layout">
    <div class="grid">
      <Field label="Density" hint="Compact rows fit more on screen.">
        {#snippet children({ id })}
          <Select
            {id}
            value={prefs.value.density}
            onchange={(event) =>
              void save(
                { density: event.currentTarget.value as Density },
                'Density saved',
              )}
          >
            <option value="comfortable">Comfortable</option>
            <option value="compact">Compact</option>
          </Select>
        {/snippet}
      </Field>
      <Field label="Start page" hint="Where Binnacle opens after sign-in.">
        {#snippet children({ id })}
          <Select
            {id}
            value={prefs.value.landingPage}
            onchange={(event) =>
              void save(
                { landingPage: event.currentTarget.value as LandingPage },
                'Start page saved',
              )}
          >
            {#each landingPages as page (page)}
              <option value={page}
                >{navigation.find((item) => item.route === page)?.label ??
                  page}</option
              >
            {/each}
          </Select>
        {/snippet}
      </Field>
      <Field label="Default chart range">
        {#snippet children({ id })}
          <Select
            {id}
            value={prefs.value.chartRange}
            onchange={(event) =>
              void save(
                { chartRange: event.currentTarget.value as ChartRange },
                'Chart range saved',
              )}
          >
            {#each chartRanges as range (range)}<option value={range}
                >{range}</option
              >{/each}
          </Select>
        {/snippet}
      </Field>
    </div>
  </Card>

  <Card
    title="Pinned resources"
    description="Pinned resources stay at the top of the Overview and Resources lists. Up to 12; missing ones are ignored."
    padded={false}
  >
    <div class="pin-add">
      <ResourcePicker
        snapshot={live}
        bind:value={pick}
        label="Resource to pin"
      />
      <Button onclick={addPin} disabled={!pick}>
        {#snippet icon()}<Pin />{/snippet}
        Pin
      </Button>
    </div>
    {#if prefs.value.pinnedResources.length}
      <ol class="pins">
        {#each prefs.value.pinnedResources as id, index (id)}
          <li>
            <span class="pin-index num">{index + 1}</span>
            <span class="pin-name"
              >{names.get(id) ?? id}{#if !names.has(id)}<span class="missing">
                  · not currently running</span
                >{/if}</span
            >
            <IconButton
              label="Move up"
              size="sm"
              disabled={index === 0 || prefs.saving}
              onclick={() => movePin(index, -1)}><ArrowUp /></IconButton
            >
            <IconButton
              label="Move down"
              size="sm"
              disabled={index === prefs.value.pinnedResources.length - 1 ||
                prefs.saving}
              onclick={() => movePin(index, 1)}><ArrowDown /></IconButton
            >
            <IconButton
              label="Unpin"
              size="sm"
              disabled={prefs.saving}
              onclick={() =>
                void save(
                  {
                    pinnedResources: prefs.value.pinnedResources.filter(
                      (value) => value !== id,
                    ),
                  },
                  'Resource unpinned',
                )}><X /></IconButton
            >
          </li>
        {/each}
      </ol>
    {:else}
      <EmptyState
        title="Nothing pinned"
        description="Pick a resource above to keep it at the top."
        compact
      >
        {#snippet icon()}<Pin />{/snippet}
      </EmptyState>
    {/if}
  </Card>
</div>

<style>
  .stack {
    display: grid;
    gap: var(--space-5);
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-4);
  }
  .pin-add {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--space-2);
    padding: var(--space-4);
    border-bottom: 1px solid var(--border);
  }
  .pins {
    margin: 0;
    padding: var(--space-2) 0;
    list-style: none;
  }
  .pins li {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-4);
    font-size: var(--text-sm);
  }
  .pin-index {
    width: 20px;
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .pin-name {
    flex: 1;
    min-width: 0;
    font-weight: 500;
  }
  .missing {
    color: var(--text-3);
    font-weight: 400;
    font-size: var(--text-xs);
  }
  @media (max-width: 900px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }
</style>
