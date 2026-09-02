<script lang="ts">
  import { onMount } from 'svelte';
  import Copy from '@lucide/svelte/icons/copy';
  import KeyRound from '@lucide/svelte/icons/key-round';
  import Plus from '@lucide/svelte/icons/plus';
  import type { SettingsSnapshot } from '../api/settings';
  import {
    coolifyStatus,
    createToken,
    listTokens,
    revokeToken,
    saveCoolify,
    scopeLabels,
    testCoolify,
    type ApiToken,
    type CoolifyStatus,
    type TokenScope,
  } from '../api/access';
  import { errorMessage } from '../api/client';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import ConfirmDialog from '../ui/ConfirmDialog.svelte';
  import DataTable, { type Column } from '../ui/DataTable.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Select from '../ui/Select.svelte';
  import StatusPill from '../ui/StatusPill.svelte';
  import { toasts } from '../ui/toast.svelte';

  let { snapshot }: { snapshot: SettingsSnapshot | null } = $props();

  /* Coolify */
  let coolify = $state<CoolifyStatus | null>(null);
  let coolifyUrl = $state('');
  let coolifyToken = $state('');
  let coolifyBusy = $state<'save' | 'test' | null>(null);
  let coolifyError = $state('');

  async function loadCoolify() {
    try {
      coolify = await coolifyStatus();
      coolifyUrl = coolify.url ?? '';
    } catch {
      coolify = null;
    }
  }

  async function runCoolify(action: 'save' | 'test') {
    coolifyBusy = action;
    coolifyError = '';
    try {
      if (action === 'test') {
        await testCoolify(coolifyUrl, coolifyToken);
        toasts.success('Coolify reachable', {
          description: 'Read access verified.',
        });
      } else {
        coolify = await saveCoolify(coolifyUrl, coolifyToken);
        coolifyToken = '';
        toasts.success('Coolify settings saved');
      }
    } catch (reason) {
      coolifyError = errorMessage(reason);
    } finally {
      coolifyBusy = null;
    }
  }

  /* API tokens */
  let tokens = $state<ApiToken[]>([]);
  let scopes = $state<TokenScope[]>([]);
  let tokensError = $state('');
  let tokenDialog = $state(false);
  let tokenName = $state('');
  let tokenScopes = $state<TokenScope[]>(['server:read']);
  let tokenExpiry = $state('none');
  let tokenBusy = $state(false);
  let tokenError = $state('');
  let plaintext = $state('');
  let revoking = $state<ApiToken | null>(null);
  let revokeOpen = $state(false);

  const portability = $derived(Boolean(snapshot?.features.portability));

  async function loadTokens() {
    if (!portability) return;
    try {
      const result = await listTokens();
      tokens = result.tokens;
      scopes = result.scopes;
    } catch (reason) {
      tokensError = errorMessage(reason);
    }
  }

  async function create() {
    tokenBusy = true;
    tokenError = '';
    try {
      if (!tokenName.trim()) throw new Error('Give the token a name.');
      if (!tokenScopes.length) throw new Error('Choose at least one scope.');
      const result = await createToken({
        name: tokenName.trim(),
        scopes: tokenScopes,
        expiresAt:
          tokenExpiry === 'none'
            ? undefined
            : new Date(
                Date.now() + Number(tokenExpiry) * 86_400_000,
              ).toISOString(),
      });
      tokens = [result.token, ...tokens];
      plaintext = result.plaintext;
      tokenName = '';
    } catch (reason) {
      tokenError = errorMessage(reason);
    } finally {
      tokenBusy = false;
    }
  }

  async function revoke() {
    if (!revoking) return;
    await revokeToken(revoking.id);
    toasts.success('Token revoked', { description: revoking.name });
    revoking = null;
    await loadTokens();
  }

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      toasts.success('Copied');
    } catch {
      toasts.error('Could not copy');
    }
  }

  onMount(() => {
    void loadCoolify();
    void loadTokens();
  });

  const tokenColumns: Column<ApiToken>[] = [
    { key: 'name', label: 'Token', cell: tokenNameCell },
    { key: 'scopes', label: 'Scopes', hideBelow: 900, cell: tokenScopesCell },
    {
      key: 'created',
      label: 'Created',
      align: 'right',
      width: '120px',
      hideBelow: 720,
      cell: tokenCreated,
    },
    {
      key: 'used',
      label: 'Last used',
      align: 'right',
      width: '120px',
      cell: tokenUsed,
    },
    {
      key: 'expires',
      label: 'Expires',
      align: 'right',
      width: '120px',
      hideBelow: 1100,
      cell: tokenExpires,
    },
    {
      key: 'actions',
      label: 'Actions',
      srOnly: true,
      align: 'right',
      width: '100px',
      cell: tokenActions,
    },
  ];

  const origin = typeof location !== 'undefined' ? location.origin : '';
</script>

{#snippet tokenNameCell(row: ApiToken)}
  <div class="token-name">
    <span class="strong">{row.name}</span>
    <code class="prefix">{row.prefix}…</code>
    {#if row.revokedAt}<Badge tone="neutral">Revoked</Badge>{/if}
  </div>
{/snippet}
{#snippet tokenScopesCell(row: ApiToken)}
  <span class="scopes"
    >{#each row.scopes as scope (scope)}<Badge tone="neutral"
        >{scopeLabels[scope] ?? scope}</Badge
      >{/each}</span
  >
{/snippet}
{#snippet tokenCreated(row: ApiToken)}<RelativeTime
    value={row.createdAt}
  />{/snippet}
{#snippet tokenUsed(row: ApiToken)}<RelativeTime
    value={row.lastUsedAt}
  />{/snippet}
{#snippet tokenExpires(row: ApiToken)}{#if row.expiresAt}<RelativeTime
      value={row.expiresAt}
    />{:else}<span class="muted">never</span>{/if}{/snippet}
{#snippet tokenActions(row: ApiToken)}
  {#if !row.revokedAt}
    <Button
      size="sm"
      variant="ghost"
      onclick={() => {
        revoking = row;
        revokeOpen = true;
      }}>Revoke</Button
    >
  {/if}
{/snippet}
{#snippet tokensEmpty()}
  <EmptyState
    title="No API tokens"
    description="Tokens give read-only access to metrics, events, incidents, and resources for scripts and Prometheus."
    compact
  >
    {#snippet icon()}<KeyRound />{/snippet}
  </EmptyState>
{/snippet}

<div class="stack">
  <Card
    title="Coolify"
    description="Read-only enrichment: project and environment names come from the Coolify API when a token is configured. Docker monitoring works without it."
  >
    {#snippet actions()}
      {#if coolify}
        <StatusPill
          status={coolify.enabled ? coolify.collector.state : 'unknown'}
          label={coolify.enabled
            ? `${coolify.collector.state} · ${coolify.collector.resources} resources`
            : 'Not configured'}
          size="sm"
        />
      {/if}
    {/snippet}
    <form
      class="form"
      onsubmit={(event) => {
        event.preventDefault();
        void runCoolify('save');
      }}
    >
      {#if coolify?.environmentAuthoritative}
        <p class="note">
          Coolify is configured through the deployment environment, so these
          fields are read-only here.
        </p>
      {/if}
      <div class="grid2">
        <Field
          label="Coolify URL"
          hint="The base URL of your Coolify instance."
        >
          {#snippet children({ id, describedBy })}
            <Input
              {id}
              type="url"
              bind:value={coolifyUrl}
              mono
              placeholder="https://coolify.example.com"
              disabled={coolify?.environmentAuthoritative}
              aria-describedby={describedBy}
            />
          {/snippet}
        </Field>
        <Field
          label="API token"
          hint={coolify?.tokenConfigured
            ? 'A token is stored. Leave blank to keep it.'
            : 'Create a read-only token in Coolify.'}
        >
          {#snippet children({ id, describedBy })}
            <Input
              {id}
              type="password"
              autocomplete="off"
              bind:value={coolifyToken}
              mono
              disabled={coolify?.environmentAuthoritative}
              aria-describedby={describedBy}
            />
          {/snippet}
        </Field>
      </div>
      {#if coolify?.collector.errorCode}
        <p class="error">
          Last sync problem: {coolify.collector.errorCode.replaceAll(
            '_',
            ' ',
          )}{#if coolify.collector.lastSuccessAt}
            · last success <RelativeTime
              value={coolify.collector.lastSuccessAt}
            />{/if}
        </p>
      {/if}
      {#if coolifyError}<p class="error" role="alert">{coolifyError}</p>{/if}
      {#if !coolify?.environmentAuthoritative}
        <div class="actions">
          <Button
            onclick={() => runCoolify('test')}
            loading={coolifyBusy === 'test'}
            disabled={!coolifyUrl}>Test connection</Button
          >
          <Button
            variant="primary"
            onclick={() => runCoolify('save')}
            loading={coolifyBusy === 'save'}
            disabled={!coolifyUrl}>Save</Button
          >
        </div>
      {/if}
    </form>
  </Card>

  <Card
    title="API tokens"
    description="Scoped, read-only bearer tokens for scripts, exports, and Prometheus. The plaintext is shown once."
    padded={false}
  >
    {#snippet actions()}
      {#if portability}
        <Button
          size="sm"
          variant="primary"
          onclick={() => {
            tokenName = '';
            tokenScopes = ['server:read'];
            tokenExpiry = 'none';
            plaintext = '';
            tokenError = '';
            tokenDialog = true;
          }}
        >
          {#snippet icon()}<Plus />{/snippet}
          New token
        </Button>
      {:else}
        <Badge tone="neutral">Disabled at deployment</Badge>
      {/if}
    {/snippet}
    {#if !portability}
      <p class="note padded">
        Portability is switched off. Set <code
          >BINNACLE_FEATURE_PORTABILITY=true</code
        > and restart to enable API tokens, exports, and Prometheus.
      </p>
    {:else if tokensError}
      <p class="error padded" role="alert">{tokensError}</p>
    {:else}
      <DataTable
        rows={tokens}
        columns={tokenColumns}
        rowKey={(row) => row.id}
        caption="API tokens"
        empty={tokensEmpty}
        density="compact"
      />
    {/if}
  </Card>

  <Card
    title="Prometheus and exports"
    description={portability
      ? 'Point Prometheus at the metrics endpoint with a token that has the history scope. Exports are bounded to 30 days and 10,000 rows.'
      : 'Available once portability is enabled.'}
  >
    {#if portability}
      <dl class="endpoints">
        <div>
          <dt>Prometheus scrape</dt>
          <dd>
            <code>{origin}/metrics</code>
            <Button
              size="sm"
              variant="ghost"
              onclick={() => copy(`${origin}/metrics`)}
              >{#snippet icon()}<Copy />{/snippet}Copy</Button
            >
          </dd>
          <dd class="muted">
            Requires <code>prometheus.enabled</code> in the deployment and an
            <code>Authorization: Bearer</code> header.
          </dd>
        </div>
        <div>
          <dt>Exports</dt>
          <dd class="muted">
            <code>/api/v1/exports/metrics.csv</code>, <code>events.json</code>,
            <code>incidents.json</code>, <code>resources.json</code>
          </dd>
        </div>
      </dl>
    {/if}
  </Card>
</div>

<Dialog
  bind:open={tokenDialog}
  title={plaintext ? 'Token created' : 'New API token'}
  description={plaintext
    ? 'Copy it now. Binnacle stores only a hash and cannot show it again.'
    : 'Tokens are read-only and limited to the scopes you choose.'}
  size="sm"
  dismissible={!tokenBusy}
>
  {#if plaintext}
    <div class="plaintext">
      <code>{plaintext}</code>
      <Button size="sm" onclick={() => copy(plaintext)}
        >{#snippet icon()}<Copy />{/snippet}Copy token</Button
      >
    </div>
  {:else}
    <form
      class="form"
      onsubmit={(event) => {
        event.preventDefault();
        void create();
      }}
    >
      <Field label="Name" required>
        {#snippet children({ id })}<Input
            {id}
            bind:value={tokenName}
            maxlength={120}
            placeholder="Prometheus scraper"
            data-autofocus
          />{/snippet}
      </Field>
      <fieldset class="scopes-field">
        <legend>Scopes</legend>
        {#each scopes as scope (scope)}
          <label class="scope">
            <input
              type="checkbox"
              checked={tokenScopes.includes(scope)}
              onchange={(event) =>
                (tokenScopes = event.currentTarget.checked
                  ? [...tokenScopes, scope]
                  : tokenScopes.filter((value) => value !== scope))}
            />
            <span>{scopeLabels[scope] ?? scope}</span>
            <code>{scope}</code>
          </label>
        {/each}
      </fieldset>
      <Field label="Expires">
        {#snippet children({ id })}
          <Select {id} bind:value={tokenExpiry}>
            <option value="none">Never</option>
            <option value="30">In 30 days</option>
            <option value="90">In 90 days</option>
            <option value="365">In a year</option>
          </Select>
        {/snippet}
      </Field>
      {#if tokenError}<p class="error" role="alert">{tokenError}</p>{/if}
    </form>
  {/if}
  {#snippet footer()}
    {#if plaintext}
      <Button variant="primary" onclick={() => (tokenDialog = false)}
        >Done</Button
      >
    {:else}
      <Button
        variant="ghost"
        onclick={() => (tokenDialog = false)}
        disabled={tokenBusy}>Cancel</Button
      >
      <Button variant="primary" onclick={create} loading={tokenBusy}
        >Create token</Button
      >
    {/if}
  {/snippet}
</Dialog>

<ConfirmDialog
  bind:open={revokeOpen}
  title="Revoke this token?"
  description={revoking ? `${revoking.name} stops working immediately.` : ''}
  confirmLabel="Revoke token"
  onconfirm={revoke}
/>

<style>
  .stack {
    display: grid;
    gap: var(--space-5);
  }
  .form {
    display: grid;
    gap: var(--space-4);
  }
  .grid2 {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }
  .actions {
    display: flex;
    gap: var(--space-2);
  }
  .note {
    color: var(--text-2);
    font-size: var(--text-sm);
  }
  .note code,
  .endpoints code {
    font-size: var(--text-xs);
  }
  .padded {
    padding: var(--space-4);
  }
  .error {
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  .token-name {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .strong {
    font-weight: 600;
  }
  .prefix {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .scopes {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
  .muted {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .endpoints {
    display: grid;
    gap: var(--space-3);
    margin: 0;
    font-size: var(--text-sm);
  }
  .endpoints div {
    display: grid;
    gap: 4px;
  }
  .endpoints dt {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .endpoints dd {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
  }
  .plaintext {
    display: grid;
    gap: var(--space-3);
  }
  .plaintext code {
    padding: var(--space-3);
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    font-size: var(--text-xs);
    overflow-wrap: anywhere;
  }
  .scopes-field {
    display: grid;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    border: 0;
  }
  .scopes-field legend {
    margin-bottom: var(--space-1);
    font-size: var(--text-sm);
    font-weight: 500;
  }
  .scope {
    display: grid;
    grid-template-columns: auto 1fr auto;
    gap: var(--space-2);
    align-items: center;
    font-size: var(--text-sm);
  }
  .scope code {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  @media (max-width: 720px) {
    .grid2 {
      grid-template-columns: 1fr;
    }
  }
</style>
