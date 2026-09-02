<script lang="ts">
  import { onMount } from 'svelte';
  import BellOff from '@lucide/svelte/icons/bell-off';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import {
    listAlerts,
    listIncidents,
    type Alert,
    type Incident,
  } from '../api/incidents';
  import {
    deleteSilence,
    listSilences,
    silenceActive,
    type Silence,
  } from '../api/silences';
  import { errorMessage } from '../api/client';
  import { incidentPath } from '../router';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import StatusPill from '../ui/StatusPill.svelte';
  import { toasts } from '../ui/toast.svelte';
  import { severityLabel, severityTone } from '../ui/status';

  let {
    resourceId,
    project,
    onsilence,
    refreshKey = 0,
  }: {
    resourceId: string;
    project?: string;
    onsilence?: () => void;
    refreshKey?: number;
  } = $props();

  let incidents = $state<Incident[]>([]);
  let alerts = $state<Alert[]>([]);
  let silences = $state<Silence[]>([]);
  let loading = $state(true);
  let error = $state('');

  async function load() {
    error = '';
    try {
      const [allIncidents, resourceAlerts, allSilences] = await Promise.all([
        listIncidents({ limit: 100 }),
        listAlerts({ resource: resourceId, limit: 50 }),
        listSilences(true),
      ]);
      incidents = allIncidents.filter(
        (incident) =>
          incident.targetType === 'resource' &&
          incident.targetId === resourceId,
      );
      alerts = resourceAlerts;
      silences = allSilences.filter(
        (silence) =>
          silenceActive(silence) &&
          (silence.scopeType === 'server' ||
            (silence.scopeType === 'resource' &&
              silence.scopeId === resourceId) ||
            (silence.scopeType === 'project' && silence.scopeId === project)),
      );
    } catch (reason) {
      error = errorMessage(reason);
    } finally {
      loading = false;
    }
  }

  async function endSilence(silence: Silence) {
    try {
      await deleteSilence(silence.id);
      toasts.success('Silence removed');
      await load();
    } catch (reason) {
      toasts.error('Silence could not be removed', {
        description: errorMessage(reason),
      });
    }
  }

  onMount(() => {
    void load();
  });

  $effect(() => {
    void refreshKey;
    void load();
  });

  const open = $derived(
    incidents.filter((incident) => incident.status === 'open'),
  );
  const resolved = $derived(
    incidents.filter((incident) => incident.status !== 'open').slice(0, 10),
  );
  const firing = $derived(alerts.filter((alert) => alert.status === 'firing'));
</script>

<div class="stack">
  {#if loading}
    <Skeleton height={140} />
  {:else if error}
    <p class="error" role="alert">{error}</p>
  {:else}
    <Card title="Incidents" padded={false}>
      {#if !incidents.length}
        <EmptyState
          title="No incidents for this resource"
          description="Firing alerts group into incidents here."
          compact
          tone="ok"
        >
          {#snippet icon()}<ShieldCheck />{/snippet}
        </EmptyState>
      {:else}
        <ul class="list">
          {#each [...open, ...resolved] as incident (incident.id)}
            <li>
              <StatusPill
                status={incident.status === 'open'
                  ? incident.severity === 'critical'
                    ? 'down'
                    : 'degraded'
                  : 'healthy'}
                label={incident.status === 'open'
                  ? severityLabel(incident.severity)
                  : 'Resolved'}
              />
              <div class="text">
                <a href={incidentPath(incident.id)}>{incident.title}</a>
                <span class="meta">
                  {incident.firingAlertCount} firing of {incident.alertCount} · opened
                  <RelativeTime value={incident.openedAt} />
                  {#if incident.resolvedAt}· resolved <RelativeTime
                      value={incident.resolvedAt}
                    />{/if}
                </span>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </Card>

    <Card title="Firing alerts" padded={false}>
      {#if !firing.length}
        <EmptyState
          title="No alerts firing"
          description="Alert rules for this resource are within thresholds."
          compact
          tone="ok"
        >
          {#snippet icon()}<ShieldCheck />{/snippet}
        </EmptyState>
      {:else}
        <ul class="list">
          {#each firing as alert (alert.id)}
            <li>
              <Badge tone={severityTone(alert.severity)} dot
                >{severityLabel(alert.severity)}</Badge
              >
              <div class="text">
                <span>{alert.message}</span>
                <span class="meta"
                  >{alert.family.replaceAll('_', ' ')} · since <RelativeTime
                    value={alert.startedAt}
                  />{#if alert.observedValue != null}· observed {alert.observedValue}{/if}</span
                >
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </Card>

    <Card title="Active silences" padded={false}>
      {#snippet actions()}
        {#if onsilence}<Button size="sm" onclick={onsilence}>Silence…</Button
          >{/if}
      {/snippet}
      {#if !silences.length}
        <EmptyState
          title="Notifications are active"
          description="No silence covers this resource right now."
          compact
        >
          {#snippet icon()}<BellOff />{/snippet}
        </EmptyState>
      {:else}
        <ul class="list">
          {#each silences as silence (silence.id)}
            <li>
              <Badge tone="info"
                >{silence.scopeType === 'server'
                  ? 'Server'
                  : silence.scopeType === 'project'
                    ? `Project ${silence.scopeId}`
                    : 'This resource'}</Badge
              >
              <div class="text">
                <span>{silence.reason}</span>
                <span class="meta"
                  >ends <RelativeTime value={silence.endsAt} /></span
                >
              </div>
              <Button
                size="sm"
                variant="ghost"
                onclick={() => endSilence(silence)}>End now</Button
              >
            </li>
          {/each}
        </ul>
      {/if}
    </Card>
  {/if}
</div>

<style>
  .stack {
    display: grid;
    gap: var(--space-4);
  }
  .error {
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  .list {
    margin: 0;
    padding: var(--space-2) 0;
    list-style: none;
  }
  .list li {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-4);
  }
  .list li + li {
    border-top: 1px solid var(--border);
  }
  .text {
    display: grid;
    flex: 1;
    gap: 2px;
    min-width: 0;
    font-size: var(--text-sm);
  }
  .text a {
    font-weight: 500;
  }
  .meta {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
</style>
