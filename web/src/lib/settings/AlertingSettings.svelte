<script lang="ts">
  import { onMount } from 'svelte';
  import type { LiveSnapshot } from '../live.svelte';
  import { listRules, type Rule } from '../api/alerts';
  import { listChecks } from '../api/checks';
  import ChecksTab from '../alerts/ChecksTab.svelte';
  import RulesTab from '../alerts/RulesTab.svelte';
  import SourceState from '../alerts/SourceState.svelte';
  import { TabSource } from '../alerts/tab-source.svelte';
  import Card from '../ui/Card.svelte';

  /**
   * Settings › Alerting: what decides that something fires. Rules and checks
   * are configuration, so they live here; live check results and incidents
   * stay operational under Incidents (DESIGN §IA).
   */
  let { live = null }: { live?: LiveSnapshot | null } = $props();

  const rules = new TabSource((signal) => listRules(signal));
  const checks = new TabSource((signal) => listChecks(signal));
  let controller: AbortController | undefined;

  const names = $derived(
    new Map(
      (live?.resources ?? []).map((resource) => [resource.id, resource.name]),
    ),
  );

  /** A saved rule replaces its row; nothing else needs refetching. */
  function ruleSaved(saved: Rule) {
    rules.data = (rules.data ?? []).map((rule) =>
      rule.id === saved.id ? saved : rule,
    );
  }

  onMount(() => {
    controller = new AbortController();
    void rules.load(controller.signal);
    void checks.load(controller.signal);
    return () => controller?.abort();
  });
</script>

<div class="stack">
  <Card
    title="Alert rules"
    description="Thresholds and conditions that open incidents. The gauges on Headroom take their warn and critical marks from these rules."
    padded={false}
  >
    <SourceState
      source={rules}
      what="Alert rules"
      onretry={() => void rules.load(controller?.signal)}
    >
      <RulesTab
        rules={rules.data ?? []}
        loading={rules.loading}
        onchanged={ruleSaved}
      />
    </SourceState>
  </Card>

  <Card
    title="Health checks"
    description="HTTP probes run from this server. Their live results are under Incidents › Checks."
    padded={false}
  >
    <SourceState
      source={checks}
      what="Health checks"
      onretry={() => void checks.load(controller?.signal)}
    >
      <ChecksTab
        checks={checks.data ?? []}
        loading={checks.loading}
        snapshot={live}
        {names}
        onchanged={() => void checks.load(controller?.signal)}
      />
    </SourceState>
  </Card>
</div>

<style>
  .stack {
    display: grid;
    gap: var(--space-4);
  }
</style>
