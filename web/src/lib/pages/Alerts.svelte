<script lang="ts">
  import { onMount } from 'svelte';
  import BellOff from '@lucide/svelte/icons/bell-off';
  import type { LiveStore } from '../live.svelte';
  import { listRules, type Rule } from '../api/alerts';
  import { listChecks, type Check } from '../api/checks';
  import {
    listAlerts,
    listIncidents,
    type Alert,
    type Incident,
  } from '../api/incidents';
  import {
    listSilences,
    type Silence,
    type SilenceScope,
  } from '../api/silences';
  import { errorMessage } from '../api/client';
  import { router } from '../router.svelte';
  import { shell } from '../shell/shell-state.svelte';
  import ActiveAlertsTab from '../alerts/ActiveAlertsTab.svelte';
  import ChecksTab from '../alerts/ChecksTab.svelte';
  import IncidentsTab from '../alerts/IncidentsTab.svelte';
  import RulesTab from '../alerts/RulesTab.svelte';
  import SilenceDialog from '../alerts/SilenceDialog.svelte';
  import SilencesTab from '../alerts/SilencesTab.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import Tabs, { tabPanelId, tabId } from '../ui/Tabs.svelte';
  import { toasts } from '../ui/toast.svelte';

  let { live }: { live: LiveStore } = $props();

  let tab = $state('incidents');
  let incidents = $state<Incident[]>([]);
  let alerts = $state<Alert[]>([]);
  let rules = $state<Rule[]>([]);
  let checks = $state<Check[]>([]);
  let silences = $state<Silence[]>([]);
  let loading = $state(true);
  let silenceOpen = $state(false);
  let silenceScope = $state<SilenceScope>('server');
  let silenceScopeId = $state('');

  const names = $derived(
    new Map(
      (live.snapshot?.resources ?? []).map((resource) => [
        resource.id,
        resource.name,
      ]),
    ),
  );
  const ruleNames = $derived(
    new Map(rules.map((rule) => [rule.id, rule.name])),
  );
  const openCount = $derived(
    incidents.filter((incident) => incident.status === 'open').length,
  );

  async function load(quiet = false) {
    if (!quiet) loading = true;
    try {
      const [nextIncidents, nextAlerts, nextRules, nextChecks, nextSilences] =
        await Promise.all([
          listIncidents({ limit: 100 }),
          listAlerts({ status: 'firing', limit: 100 }),
          listRules(),
          listChecks(),
          listSilences(),
        ]);
      incidents = nextIncidents;
      alerts = nextAlerts;
      rules = nextRules;
      checks = nextChecks;
      silences = nextSilences;
      shell.reportOpenIncidents(
        incidents.filter((incident) => incident.status === 'open').length,
      );
    } catch (reason) {
      if (!quiet)
        toasts.error('Alert data could not be loaded', {
          description: errorMessage(reason),
        });
    } finally {
      loading = false;
    }
  }

  function openSilence(scope: SilenceScope = 'server', scopeId = '') {
    silenceScope = scope;
    silenceScopeId = scopeId;
    silenceOpen = true;
  }

  onMount(() => {
    void load();
    if (router.param('new') === '1') {
      openSilence();
      router.setQuery({ new: null });
    }
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') void load(true);
    }, 15_000);
    return () => window.clearInterval(timer);
  });
</script>

<PageHeader
  title="Alerts"
  description="Incidents group firing alerts by target. Rules, checks, and silences decide what fires and who hears about it."
>
  {#snippet actions()}
    <Button onclick={() => openSilence()}>
      {#snippet icon()}<BellOff />{/snippet}
      Silence…
    </Button>
  {/snippet}
  <Tabs
    prefix="alerts"
    label="Alert sections"
    param="tab"
    bind:active={tab}
    tabs={[
      { id: 'incidents', label: 'Incidents', count: openCount },
      { id: 'active', label: 'Firing', count: alerts.length },
      { id: 'rules', label: 'Rules', count: rules.length || null },
      { id: 'checks', label: 'Checks', count: checks.length || null },
      {
        id: 'silences',
        label: 'Silences',
        count:
          silences.filter((silence) => Date.parse(silence.endsAt) > Date.now())
            .length || null,
      },
    ]}
  />
</PageHeader>

<div
  id={tabPanelId('alerts', 'incidents')}
  role="tabpanel"
  aria-labelledby={tabId('alerts', 'incidents')}
  hidden={tab !== 'incidents'}
>
  {#if tab === 'incidents'}
    <Card padded={false}><IncidentsTab {incidents} {loading} {names} /></Card>
  {/if}
</div>
<div
  id={tabPanelId('alerts', 'active')}
  role="tabpanel"
  aria-labelledby={tabId('alerts', 'active')}
  hidden={tab !== 'active'}
>
  {#if tab === 'active'}
    <Card padded={false}
      ><ActiveAlertsTab
        {alerts}
        {loading}
        {names}
        onsilence={openSilence}
        onchanged={() => void load(true)}
      /></Card
    >
  {/if}
</div>
<div
  id={tabPanelId('alerts', 'rules')}
  role="tabpanel"
  aria-labelledby={tabId('alerts', 'rules')}
  hidden={tab !== 'rules'}
>
  {#if tab === 'rules'}
    <Card padded={false}
      ><RulesTab {rules} {loading} onchanged={() => void load(true)} /></Card
    >
  {/if}
</div>
<div
  id={tabPanelId('alerts', 'checks')}
  role="tabpanel"
  aria-labelledby={tabId('alerts', 'checks')}
  hidden={tab !== 'checks'}
>
  {#if tab === 'checks'}
    <Card padded={false}
      ><ChecksTab
        {checks}
        {loading}
        snapshot={live.snapshot}
        {names}
        onchanged={() => void load(true)}
      /></Card
    >
  {/if}
</div>
<div
  id={tabPanelId('alerts', 'silences')}
  role="tabpanel"
  aria-labelledby={tabId('alerts', 'silences')}
  hidden={tab !== 'silences'}
>
  {#if tab === 'silences'}
    <Card padded={false}
      ><SilencesTab
        {silences}
        {loading}
        {names}
        rules={ruleNames}
        oncreate={() => openSilence()}
        onchanged={() => void load(true)}
      /></Card
    >
  {/if}
</div>

<SilenceDialog
  bind:open={silenceOpen}
  snapshot={live.snapshot}
  scope={silenceScope}
  scopeId={silenceScopeId}
  rules={rules.map((rule) => ({ id: rule.id, name: rule.name }))}
  oncreated={() => void load(true)}
/>
