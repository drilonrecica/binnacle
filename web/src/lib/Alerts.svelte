<script lang="ts">
  import { onMount } from 'svelte';
  import { csrfToken } from './auth';
  import Alert from './ui/Alert.svelte';
  import ConsoleSection from './ui/ConsoleSection.svelte';
  import EmptyState from './ui/EmptyState.svelte';
  type Item = {
    id: string;
    severity?: string;
    family?: string;
    targetId?: string;
    message?: string;
    startedAt?: string;
    observedValue?: number;
    name?: string;
    scopeType?: string;
    enabled?: boolean;
    resourceId?: string;
    url?: string;
    method?: string;
    required?: boolean;
    scopeId?: string;
    reason?: string;
    endsAt?: string;
    threshold?: number;
    recoveryThreshold?: number;
    triggerSeconds?: number;
    recoverySeconds?: number;
    interval?: number;
    timeout?: number;
    expectedStatusMin?: number;
    expectedStatusMax?: number;
    bodySubstring?: string;
    status?: string;
    title?: string;
    alertCount?: number;
    firingAlertCount?: number;
    openedAt?: string;
    resolvedAt?: string;
    updatedAt?: string;
    targetType?: string;
    kind?: string;
    minimumSeverity?: string;
    notifyResolved?: boolean;
    secretConfigured?: boolean;
    channelId?: string;
    incidentId?: string;
    eventType?: string;
    attemptCount?: number;
    failureCode?: string;
    idempotencyKey?: string;
    alerts?: Item[];
    deliveries?: Item[];
  };
  type Tab =
    | 'incidents'
    | 'alerts'
    | 'rules'
    | 'checks'
    | 'silences'
    | 'channels'
    | 'deliveries';
  let tab = $state<Tab>('incidents');
  let active = $state<Item[]>([]),
    incidents = $state<Item[]>([]),
    rules = $state<Item[]>([]),
    checks = $state<Item[]>([]),
    silences = $state<Item[]>([]),
    channels = $state<Item[]>([]),
    deliveries = $state<Item[]>([]);
  let selectedIncident = $state<Item | null>(null);
  let error = $state(''),
    resourceId = $state(''),
    name = $state(''),
    url = $state(''),
    required = $state(true);
  let channelName = $state(''),
    channelKind = $state('webhook'),
    channelTarget = $state(''),
    channelSender = $state(''),
    channelRecipients = $state(''),
    channelUsername = $state(''),
    channelPassword = $state(''),
    channelBearer = $state(''),
    channelSigningSecret = $state(''),
    channelTLS = $state('starttls'),
    channelCriticalOnly = $state(false);
  let silenceScope = $state('server'),
    silenceScopeId = $state(''),
    silenceReason = $state(''),
    silencePreset = $state('1h');
  async function request<T>(
    path: string,
    init?: Parameters<typeof fetch>[1],
  ): Promise<T> {
    const headers = new Headers(init?.headers);
    if (init?.body) headers.set('Content-Type', 'application/json');
    if (init?.method && init.method !== 'GET')
      headers.set('X-CSRF-Token', decodeURIComponent(csrfToken()));
    const response = await fetch(path, {
      credentials: 'same-origin',
      ...init,
      headers,
    });
    if (!response.ok) {
      const body = (await response.json().catch(() => null)) as {
        error?: { message?: string };
      } | null;
      throw new Error(
        body?.error?.message ?? 'The alerts service is unavailable.',
      );
    }
    return response.status === 204
      ? (undefined as T)
      : (response.json() as Promise<T>);
  }
  async function load() {
    error = '';
    try {
      [incidents, active, rules, checks, silences, channels, deliveries] =
        await Promise.all([
          request<Item[]>('/api/v1/incidents'),
          request<Item[]>('/api/v1/alerts?status=firing'),
          request<Item[]>('/api/v1/alert-rules'),
          request<Item[]>('/api/v1/checks'),
          request<Item[]>('/api/v1/silences'),
          request<Item[]>('/api/v1/notification-channels'),
          request<Item[]>('/api/v1/notification-deliveries'),
        ]);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Alerts are unavailable.';
    }
  }
  async function loadIncidents() {
    if (document.visibilityState !== 'visible') return;
    try {
      incidents = await request<Item[]>('/api/v1/incidents');
    } catch {
      // The full refresh action reports errors; polling remains quiet.
    }
  }
  async function inspectIncident(id: string) {
    try {
      selectedIncident = await request<Item>(
        `/api/v1/incidents/${encodeURIComponent(id)}`,
      );
    } catch (e) {
      error = e instanceof Error ? e.message : 'Incident is unavailable.';
    }
  }
  async function createChannel() {
    const smtp = channelKind === 'smtp';
    await mutate('/api/v1/notification-channels', 'POST', {
      name: channelName,
      kind: channelKind,
      enabled: true,
      minimumSeverity: channelCriticalOnly ? 'critical' : 'warning',
      notifyResolved: true,
      ...(smtp
        ? {
            host: channelTarget,
            sender: channelSender,
            recipients: channelRecipients
              .split(',')
              .map((value) => value.trim())
              .filter(Boolean),
            tlsMode: channelTLS,
            username: channelUsername,
            password: channelPassword,
          }
        : {
            url: channelTarget,
            bearerToken: channelBearer,
            signingSecret: channelSigningSecret,
          }),
    });
    channelName = '';
    channelTarget = '';
    channelSender = '';
    channelRecipients = '';
    channelUsername = '';
    channelPassword = '';
    channelBearer = '';
    channelSigningSecret = '';
  }
  async function createCheck() {
    try {
      await request('/api/v1/checks', {
        method: 'POST',
        body: JSON.stringify({
          resourceId,
          name,
          url,
          method: 'GET',
          intervalSeconds: 30,
          timeoutSeconds: 5,
          expectedStatusMin: 200,
          expectedStatusMax: 399,
          required,
          enabled: true,
        }),
      });
      resourceId = '';
      name = '';
      url = '';
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Check could not be created.';
    }
  }
  async function mutate(path: string, method: string, body?: unknown) {
    try {
      await request(path, {
        method,
        body: body ? JSON.stringify(body) : undefined,
      });
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Update failed.';
    }
  }
  async function createSilence() {
    await mutate('/api/v1/silences', 'POST', {
      scopeType: silenceScope,
      scopeId: silenceScopeId,
      reason: silenceReason,
      preset: silencePreset,
    });
    silenceReason = '';
  }
  async function silenceAlert(item: Item) {
    await mutate('/api/v1/silences', 'POST', {
      scopeType: item.targetId === 'server' ? 'server' : 'resource',
      scopeId: item.targetId === 'server' ? '' : item.targetId,
      reason: `Silence ${item.message ?? item.family ?? 'alert'}`,
      preset: '1h',
    });
  }
  async function toggleCheck(check: Item) {
    await mutate(`/api/v1/checks/${encodeURIComponent(check.id)}`, 'PATCH', {
      resourceId: check.resourceId,
      name: check.name,
      url: check.url,
      method: check.method,
      intervalSeconds: Math.round(
        (check.interval ?? 30_000_000_000) / 1_000_000_000,
      ),
      timeoutSeconds: Math.round(
        (check.timeout ?? 5_000_000_000) / 1_000_000_000,
      ),
      expectedStatusMin: check.expectedStatusMin ?? 200,
      expectedStatusMax: check.expectedStatusMax ?? 399,
      bodySubstring: check.bodySubstring ?? '',
      required: check.required,
      enabled: !check.enabled,
    });
  }
  const age = (start?: string) => {
    if (!start) return '—';
    const m = Math.max(
      0,
      Math.floor((Date.now() - new Date(start).getTime()) / 60000),
    );
    return m < 60 ? `${m}m` : `${Math.floor(m / 60)}h`;
  };
  const localTime = (value?: string) =>
    value ? new Date(value).toLocaleString() : '—';
  onMount(() => {
    void load();
    const timer = window.setInterval(() => void loadIncidents(), 15_000);
    return () => window.clearInterval(timer);
  });
</script>

<section class="console-page" aria-labelledby="alerts-title">
  <ConsoleSection
    code="ALT"
    title="Alerts"
    id="alerts-title"
    detail={`${incidents.filter((item) => item.status === 'open').length} open incidents`}
  />
  <div class="control-rail" role="tablist" aria-label="Alerts sections">
    {#each ['incidents', 'alerts', 'rules', 'checks', 'silences', 'channels', 'deliveries'] as item}<button
        role="tab"
        aria-selected={tab === item}
        onclick={() => (tab = item as Tab)}>{item}</button
      >{/each}
  </div>
  <button onclick={() => load()}>Refresh</button>
  {#if error}<Alert level="error">{error}</Alert>{/if}
  {#if tab === 'incidents'}
    {#if selectedIncident}<section
        class="card"
        aria-labelledby="incident-detail-title"
      >
        <h2 id="incident-detail-title">{selectedIncident.title}</h2>
        <p>
          {selectedIncident.status} · {selectedIncident.severity} · {selectedIncident.targetType}:{selectedIncident.targetId}
        </p>
        <button onclick={() => (selectedIncident = null)}>Close details</button>
        <div class="table-scroll">
          <table class="console-table">
            <caption>Incident member alerts</caption><thead
              ><tr
                ><th>Status</th><th>Severity</th><th>Alert</th><th>Started</th
                ></tr
              ></thead
            ><tbody
              >{#each selectedIncident.alerts ?? [] as member (member.id)}<tr
                  ><td>{member.status}</td><td>{member.severity}</td><td
                    >{member.message}</td
                  ><td>{localTime(member.startedAt)}</td></tr
                >{/each}</tbody
            >
          </table>
        </div>
        <p>
          {(selectedIncident.deliveries ?? []).length} related notification
          {(selectedIncident.deliveries ?? []).length === 1
            ? 'delivery'
            : 'deliveries'}
        </p>
      </section>{/if}
    {#if !incidents.length}<EmptyState title="No incidents"
        ><p>
          Firing alerts will be grouped here by affected target.
        </p></EmptyState
      >{:else}<div class="table-scroll">
        <table class="console-table">
          <caption>Incidents</caption><thead
            ><tr
              ><th>Status</th><th>Severity</th><th>Incident</th><th>Target</th
              ><th>Alerts</th><th>Opened</th><th>Action</th></tr
            ></thead
          ><tbody
            >{#each incidents as item (item.id)}<tr
                ><td>{item.status}</td><td>{item.severity}</td><td
                  >{item.title}</td
                ><td>{item.targetType}:{item.targetId}</td><td
                  >{item.firingAlertCount ?? 0} firing / {item.alertCount ??
                    0}</td
                ><td>{age(item.openedAt)}</td><td
                  ><button onclick={() => inspectIncident(item.id)}
                    >View details</button
                  ></td
                ></tr
              >{/each}</tbody
          >
        </table>
      </div>{/if}
  {:else if tab === 'alerts'}
    {#if !active.length}<EmptyState title="No active alerts"
        ><p>All evaluated conditions are healthy.</p></EmptyState
      >{:else}<div class="table-scroll">
        <table class="console-table">
          <caption>Active alerts</caption><thead
            ><tr
              ><th>Severity</th><th>Alert</th><th>Scope</th><th>Duration</th><th
                >Observed</th
              ><th>Action</th></tr
            ></thead
          ><tbody
            >{#each active as item (item.id)}<tr
                ><td>{item.severity}</td><td>{item.message}</td><td
                  >{item.targetId}</td
                ><td>{age(item.startedAt)}</td><td
                  >{item.observedValue ?? '—'}</td
                ><td
                  ><button onclick={() => silenceAlert(item)}>Silence 1h</button
                  ></td
                ></tr
              >{/each}</tbody
          >
        </table>
      </div>{/if}
  {:else if tab === 'rules'}
    <div class="table-scroll">
      <table class="console-table">
        <caption>Alert rules</caption><thead
          ><tr
            ><th>Rule</th><th>Severity</th><th>Scope</th><th>Thresholds</th><th
              >Status</th
            ><th>Action</th></tr
          ></thead
        ><tbody
          >{#each rules as rule (rule.id)}<tr
              ><td>{rule.name}<span class="meta">{rule.family}</span></td><td
                >{rule.severity}</td
              ><td>{rule.scopeType}</td><td
                >{rule.threshold ?? '—'} / recover {rule.recoveryThreshold ??
                  '—'}</td
              ><td>{rule.enabled ? 'Enabled' : 'Disabled'}</td><td
                ><button
                  onclick={() =>
                    mutate(
                      `/api/v1/alert-rules/${encodeURIComponent(rule.id)}`,
                      'PATCH',
                      { enabled: !rule.enabled },
                    )}>{rule.enabled ? 'Disable' : 'Enable'}</button
                ></td
              ></tr
            >{/each}</tbody
        >
      </table>
    </div>
  {:else if tab === 'checks'}
    <form
      class="control-rail"
      onsubmit={(e) => {
        e.preventDefault();
        void createCheck();
      }}
    >
      <label>Resource ID<input required bind:value={resourceId} /></label><label
        >Name<input required maxlength="120" bind:value={name} /></label
      ><label
        >HTTP/HTTPS URL<input required type="url" bind:value={url} /></label
      ><label><input type="checkbox" bind:checked={required} /> Required</label
      ><button type="submit">Create check</button>
    </form>
    {#if !checks.length}<EmptyState title="No checks"
        ><p>Create the first HTTP health check above.</p></EmptyState
      >{:else}<div class="table-scroll">
        <table class="console-table">
          <caption>Health checks</caption><thead
            ><tr
              ><th>Check</th><th>Resource</th><th>Target</th><th>Class</th><th
                >Actions</th
              ></tr
            ></thead
          ><tbody
            >{#each checks as check (check.id)}<tr
                ><td>{check.name}</td><td>{check.resourceId}</td><td
                  >{check.method} {check.url}</td
                ><td
                  >{check.required ? 'Required' : 'Optional'} · {check.enabled
                    ? 'Enabled'
                    : 'Disabled'}</td
                ><td
                  ><button onclick={() => toggleCheck(check)}
                    >{check.enabled ? 'Disable' : 'Enable'}</button
                  >
                  ><button
                    onclick={() =>
                      mutate(
                        `/api/v1/checks/${encodeURIComponent(check.id)}/run`,
                        'POST',
                      )}>Run now</button
                  >
                  <button
                    onclick={() =>
                      mutate(
                        `/api/v1/checks/${encodeURIComponent(check.id)}`,
                        'DELETE',
                      )}>Delete</button
                  ></td
                ></tr
              >{/each}</tbody
          >
        </table>
      </div>{/if}
  {:else if tab === 'silences'}
    <form
      class="control-rail"
      onsubmit={(e) => {
        e.preventDefault();
        void createSilence();
      }}
    >
      <label
        >Scope<select bind:value={silenceScope}
          ><option value="server">Server</option><option value="project"
            >Project</option
          ><option value="resource">Resource</option><option value="rule"
            >Rule</option
          ></select
        ></label
      >{#if silenceScope !== 'server'}<label
          >Scope ID<input required bind:value={silenceScopeId} /></label
        >{/if}<label
        >Duration<select bind:value={silencePreset}
          ><option value="30m">30 minutes</option><option value="1h"
            >1 hour</option
          ><option value="4h">4 hours</option><option value="tomorrow"
            >Until tomorrow</option
          ></select
        ></label
      ><label
        >Reason<input
          required
          maxlength="500"
          bind:value={silenceReason}
        /></label
      ><button type="submit">Create silence</button>
    </form>
    {#if !silences.length}<EmptyState title="No silences"
        ><p>Active and expired silences will appear here.</p></EmptyState
      >{:else}<div class="table-scroll">
        <table class="console-table">
          <caption>Silences</caption><thead
            ><tr><th>Scope</th><th>Reason</th><th>Ends</th><th>Action</th></tr
            ></thead
          ><tbody
            >{#each silences as silence (silence.id)}<tr
                ><td>{silence.scopeType} {silence.scopeId ?? ''}</td><td
                  >{silence.reason}</td
                ><td><time>{localTime(silence.endsAt)}</time></td><td
                  ><button
                    onclick={() =>
                      mutate(
                        `/api/v1/silences/${encodeURIComponent(silence.id)}`,
                        'DELETE',
                      )}>Cancel</button
                  ></td
                ></tr
              >{/each}</tbody
          >
        </table>
      </div>{/if}
  {:else if tab === 'channels'}
    <form
      class="control-rail"
      onsubmit={(event) => {
        event.preventDefault();
        void createChannel();
      }}
    >
      <label
        >Name<input required maxlength="120" bind:value={channelName} /></label
      ><label
        >Type<select bind:value={channelKind}
          ><option value="webhook">HTTPS webhook</option><option value="smtp"
            >SMTP email</option
          ></select
        ></label
      ><label
        >{channelKind === 'smtp' ? 'SMTP host:port' : 'HTTPS URL'}<input
          required
          type={channelKind === 'smtp' ? 'text' : 'url'}
          bind:value={channelTarget}
        /></label
      >{#if channelKind === 'smtp'}<label
          >Sender<input
            required
            type="email"
            bind:value={channelSender}
          /></label
        ><label
          >Recipients (comma-separated)<input
            required
            bind:value={channelRecipients}
          /></label
        ><label
          >TLS<select bind:value={channelTLS}
            ><option value="starttls">STARTTLS</option><option value="implicit"
              >Implicit TLS</option
            ></select
          ></label
        ><label>Username (optional)<input bind:value={channelUsername} /></label
        ><label
          >Password (optional)<input
            type="password"
            autocomplete="new-password"
            bind:value={channelPassword}
          /></label
        >{:else}<label
          >Bearer token (optional)<input
            type="password"
            autocomplete="new-password"
            bind:value={channelBearer}
          /></label
        ><label
          >HMAC signing secret (optional)<input
            type="password"
            autocomplete="new-password"
            bind:value={channelSigningSecret}
          /></label
        >{/if}<label
        ><input type="checkbox" bind:checked={channelCriticalOnly} />
        Critical only</label
      ><button type="submit">Create channel</button>
    </form>
    {#if !channels.length}<EmptyState title="No notification channels"
        ><p>
          Configure an HTTPS webhook or TLS-protected SMTP channel.
        </p></EmptyState
      >{:else}<div class="table-scroll">
        <table class="console-table">
          <caption>Notification channels</caption><thead
            ><tr
              ><th>Channel</th><th>Type</th><th>Filter</th><th>Status</th><th
                >Actions</th
              ></tr
            ></thead
          ><tbody
            >{#each channels as channel (channel.id)}<tr
                ><td>{channel.name}</td><td>{channel.kind}</td><td
                  >{channel.minimumSeverity}+</td
                ><td
                  >{channel.enabled ? 'Enabled' : 'Disabled'} · {channel.secretConfigured
                    ? 'secret configured'
                    : 'secret unavailable'}</td
                ><td
                  ><button
                    onclick={() =>
                      mutate(
                        `/api/v1/notification-channels/${encodeURIComponent(channel.id)}/test`,
                        'POST',
                      )}>Test</button
                  ><button
                    onclick={() =>
                      mutate(
                        `/api/v1/notification-channels/${encodeURIComponent(channel.id)}`,
                        'PATCH',
                        { enabled: !channel.enabled },
                      )}>{channel.enabled ? 'Disable' : 'Enable'}</button
                  ><button
                    onclick={() =>
                      mutate(
                        `/api/v1/notification-channels/${encodeURIComponent(channel.id)}`,
                        'DELETE',
                      )}>Delete</button
                  ></td
                ></tr
              >{/each}</tbody
          >
        </table>
      </div>{/if}
  {:else}
    {#if !deliveries.length}<EmptyState title="No delivery history"
        ><p>Notification attempts will appear here.</p></EmptyState
      >{:else}<div class="table-scroll">
        <table class="console-table">
          <caption>Notification delivery history</caption><thead
            ><tr
              ><th>Status</th><th>Event</th><th>Channel</th><th>Attempts</th><th
                >Failure</th
              ><th>Action</th></tr
            ></thead
          ><tbody
            >{#each deliveries as delivery (delivery.id)}<tr
                ><td>{delivery.status}</td><td>{delivery.eventType}</td><td
                  >{delivery.channelId}</td
                ><td>{delivery.attemptCount ?? 0}</td><td
                  >{delivery.failureCode ?? '—'}</td
                ><td
                  >{#if delivery.status === 'permanent_failure'}<button
                      onclick={() =>
                        mutate(
                          `/api/v1/notification-deliveries/${encodeURIComponent(delivery.id)}/retry`,
                          'POST',
                        )}>Retry</button
                    >{/if}</td
                ></tr
              >{/each}</tbody
          >
        </table>
      </div>{/if}
  {/if}
</section>
