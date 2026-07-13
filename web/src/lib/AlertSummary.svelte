<script lang="ts">
  import { onMount } from 'svelte';
  let { resourceId = '' }: { resourceId?: string } = $props();
  type Incident = {
    id: string;
    severity: string;
    title: string;
    targetId: string;
    status: string;
    firingAlertCount: number;
  };
  type Check = {
    id: string;
    name: string;
    required: boolean;
    enabled: boolean;
    resourceId: string;
  };
  let incidents = $state<Incident[]>([]),
    checks = $state<Check[]>([]);
  onMount(() => {
    void Promise.all([
      fetch('/api/v1/incidents?status=open', {
        credentials: 'same-origin',
      }).then((r) => (r.ok ? r.json() : [])),
      resourceId
        ? fetch('/api/v1/checks', { credentials: 'same-origin' }).then((r) =>
            r.ok ? r.json() : [],
          )
        : Promise.resolve([]),
    ]).then(([a, c]) => {
      incidents = (a as Incident[]).filter(
        (incident) => !resourceId || incident.targetId === resourceId,
      );
      checks = (c as Check[]).filter((v) => v.resourceId === resourceId);
    });
  });
</script>

{#if incidents.length || checks.length}<aside
    class="card"
    aria-label={resourceId
      ? 'Resource health checks and alerts'
      : 'Active alert summary'}
  >
    <h2>{resourceId ? 'Checks and related incidents' : 'Open incidents'}</h2>
    {#if incidents.length}<ul>
        {#each incidents.slice(0, 5) as incident (incident.id)}<li>
            <strong>{incident.severity}</strong> — {incident.title}
            ({incident.firingAlertCount} firing)
          </li>{/each}
      </ul>{:else}<p>No open related incidents.</p>{/if}{#if checks.length}<p>
        {checks.filter((c) => c.enabled).length} enabled checks · {checks.filter(
          (c) => c.required,
        ).length} required
      </p>{/if}<a href="/alerts">Open Incidents</a>
  </aside>{/if}
