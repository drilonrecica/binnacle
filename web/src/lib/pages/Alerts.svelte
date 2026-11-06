<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import BellOff from '@lucide/svelte/icons/bell-off';
  import type { LiveStore } from '../live.svelte';
  import { listRules } from '../api/alerts';
  import { listChecks } from '../api/checks';
  import { listAlerts, listIncidents } from '../api/incidents';
  import {
    listSilences,
    silenceActive,
    type SilenceScope,
  } from '../api/silences';
  import { router } from '../router.svelte';
  import { shell } from '../shell/shell-state.svelte';
  import ActiveAlertsTab from '../alerts/ActiveAlertsTab.svelte';
  import CheckStatusTab from '../alerts/CheckStatusTab.svelte';
  import IncidentsTab from '../alerts/IncidentsTab.svelte';
  import SilenceDialog from '../alerts/SilenceDialog.svelte';
  import SilencesTab from '../alerts/SilencesTab.svelte';
  import SourceState from '../alerts/SourceState.svelte';
  import { refreshSources, TabSource } from '../alerts/tab-source.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import Tabs, { tabPanelId, tabId } from '../ui/Tabs.svelte';

  let { live }: { live: LiveStore } = $props();

  /** Rows asked for per incident list. */
  const pageSize = 100;
  /** How often the lists that change on their own are refreshed. */
  const pollMs = 15_000;

  // One source per tab, each loaded and failing on its own (UI-9).
  const open = new TabSource((signal) =>
    listIncidents({ status: 'open', limit: pageSize }, signal),
  );
  const resolved = new TabSource((signal) =>
    listIncidents({ status: 'resolved', limit: pageSize }, signal),
  );
  const firing = new TabSource((signal) =>
    listAlerts({ status: 'firing', limit: pageSize }, signal),
  );
  const checks = new TabSource((signal) => listChecks(signal));
  const silences = new TabSource((signal) => listSilences(false, signal));
  // Rules only name silence scopes and fill the silence dialog; they are
  // configuration, so they are read once and never polled.
  const rules = new TabSource((signal) => listRules(signal));

  let tab = $state('open');
  let silenceOpen = $state(false);
  let silenceScope = $state<SilenceScope>('server');
  let silenceScopeId = $state('');
  let controller: AbortController | undefined;

  const names = $derived(
    new Map(
      (live.snapshot?.resources ?? []).map((resource) => [
        resource.id,
        resource.name,
      ]),
    ),
  );
  const ruleList = $derived(
    (rules.data ?? []).map((rule) => ({ id: rule.id, name: rule.name })),
  );
  const ruleNames = $derived(
    new Map(ruleList.map((rule) => [rule.id, rule.name])),
  );
  const activeSilences = $derived(
    silences.data?.filter((silence) => silenceActive(silence)).length,
  );

  /**
   * What a refresh fetches: incidents, firing alerts and check results change
   * on their own; resolved incidents and silences only matter while their
   * tab is open; rules are configuration and stay as loaded.
   */
  function polled(): TabSource<unknown>[] {
    const sources: TabSource<unknown>[] = [open, firing, checks];
    if (tab === 'resolved') sources.push(resolved);
    if (tab === 'silences') sources.push(silences);
    return sources;
  }

  async function refresh(sources: TabSource<unknown>[]) {
    const signal = controller?.signal;
    await refreshSources(sources, signal);
    if (signal?.aborted) return;
    if (sources.includes(open) && open.data && !open.error)
      shell.reportOpenIncidents(open.data.length);
  }

  function openSilence(scope: SilenceScope = 'server', scopeId = '') {
    silenceScope = scope;
    silenceScopeId = scopeId;
    silenceOpen = true;
  }

  // Older links named the tabs differently.
  $effect(() => {
    const requested = router.param('tab');
    if (requested === 'incidents')
      untrack(() => router.setQuery({ tab: null }));
    else if (requested === 'active')
      untrack(() => router.setQuery({ tab: 'firing' }));
  });

  // Watched, not read once on mount: the palette's "Create a silence…" links
  // here with new=1 while this page may already be open (UI-20).
  $effect(() => {
    if (router.param('new') !== '1') return;
    untrack(() => {
      openSilence();
      router.setQuery({ new: null });
    });
  });

  function selected(id: string) {
    if (id === 'resolved') void refresh([resolved]);
    else if (id === 'silences') void refresh([silences]);
  }

  onMount(() => {
    controller = new AbortController();
    void refresh([open, resolved, firing, checks, silences, rules]);
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') void refresh(polled());
    }, pollMs);
    return () => {
      window.clearInterval(timer);
      controller?.abort();
    };
  });
</script>

<PageHeader
  title="Incidents"
  description="Incidents group firing alerts by target. Rules and checks that decide what fires are configured under Settings › Alerting."
>
  {#snippet actions()}
    <Button onclick={() => openSilence()}>
      {#snippet icon()}<BellOff />{/snippet}
      Silence…
    </Button>
  {/snippet}
  <Tabs
    prefix="incidents"
    label="Incident sections"
    param="tab"
    bind:active={tab}
    onchange={selected}
    tabs={[
      { id: 'open', label: 'Open', count: open.data?.length ?? null },
      { id: 'resolved', label: 'Resolved' },
      { id: 'firing', label: 'Firing', count: firing.data?.length ?? null },
      { id: 'checks', label: 'Checks', count: checks.data?.length || null },
      { id: 'silences', label: 'Silences', count: activeSilences || null },
    ]}
  />
</PageHeader>

<div
  id={tabPanelId('incidents', 'open')}
  role="tabpanel"
  aria-labelledby={tabId('incidents', 'open')}
  hidden={tab !== 'open'}
>
  {#if tab === 'open'}
    <Card padded={false}>
      <SourceState
        source={open}
        what="Open incidents"
        onretry={() => void refresh([open])}
      >
        <IncidentsTab
          mode="open"
          incidents={open.data ?? []}
          loading={open.loading}
          limit={pageSize}
          {names}
        />
      </SourceState>
    </Card>
  {/if}
</div>
<div
  id={tabPanelId('incidents', 'resolved')}
  role="tabpanel"
  aria-labelledby={tabId('incidents', 'resolved')}
  hidden={tab !== 'resolved'}
>
  {#if tab === 'resolved'}
    <Card padded={false}>
      <SourceState
        source={resolved}
        what="Resolved incidents"
        onretry={() => void refresh([resolved])}
      >
        <IncidentsTab
          mode="resolved"
          incidents={resolved.data ?? []}
          loading={resolved.loading}
          limit={pageSize}
          {names}
        />
      </SourceState>
    </Card>
  {/if}
</div>
<div
  id={tabPanelId('incidents', 'firing')}
  role="tabpanel"
  aria-labelledby={tabId('incidents', 'firing')}
  hidden={tab !== 'firing'}
>
  {#if tab === 'firing'}
    <Card padded={false}>
      <SourceState
        source={firing}
        what="Firing alerts"
        onretry={() => void refresh([firing])}
      >
        <ActiveAlertsTab
          alerts={firing.data ?? []}
          loading={firing.loading}
          {names}
          onsilence={openSilence}
          onchanged={() => void refresh([firing, silences])}
        />
      </SourceState>
    </Card>
  {/if}
</div>
<div
  id={tabPanelId('incidents', 'checks')}
  role="tabpanel"
  aria-labelledby={tabId('incidents', 'checks')}
  hidden={tab !== 'checks'}
>
  {#if tab === 'checks'}
    <Card padded={false}>
      <SourceState
        source={checks}
        what="Health checks"
        onretry={() => void refresh([checks])}
      >
        <CheckStatusTab
          checks={checks.data ?? []}
          loading={checks.loading}
          {names}
        />
      </SourceState>
    </Card>
  {/if}
</div>
<div
  id={tabPanelId('incidents', 'silences')}
  role="tabpanel"
  aria-labelledby={tabId('incidents', 'silences')}
  hidden={tab !== 'silences'}
>
  {#if tab === 'silences'}
    <Card padded={false}>
      <SourceState
        source={silences}
        what="Silences"
        onretry={() => void refresh([silences])}
      >
        <SilencesTab
          silences={silences.data ?? []}
          loading={silences.loading}
          {names}
          rules={ruleNames}
          oncreate={() => openSilence()}
          onchanged={() => void refresh([silences])}
        />
      </SourceState>
    </Card>
  {/if}
</div>

<SilenceDialog
  bind:open={silenceOpen}
  snapshot={live.snapshot}
  scope={silenceScope}
  scopeId={silenceScopeId}
  rules={ruleList}
  oncreated={() => void refresh([silences, firing])}
/>
