// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render, screen, waitFor } from '@testing-library/svelte';
import { sessionEndedEvent } from './lib/api/client';
import { router } from './lib/router.svelte';
import { fakeLive } from './lib/shell/testing/fake-live.svelte';

const mocks = vi.hoisted(() => ({
  currentSession: vi.fn(),
  sessionActive: vi.fn(),
  setupAvailable: vi.fn(),
}));

const stub = vi.hoisted(() => async () => ({
  default: (await import('./lib/shell/testing/PageStub.test.svelte')).default,
}));

vi.mock('./lib/auth', () => ({
  currentSession: mocks.currentSession,
  authMethods: vi.fn().mockResolvedValue({ proxyAvailable: false }),
  bootstrapExternalSession: vi.fn(),
}));
vi.mock('./lib/onboarding', () => ({
  onboardingState: vi
    .fn()
    .mockResolvedValue({ completedAt: '2026-11-01T00:00:00Z' }),
  setupAvailable: mocks.setupAvailable,
}));
vi.mock('./lib/live.svelte', async () => {
  const { FakeLive, fakeLive: holder } =
    await import('./lib/shell/testing/fake-live.svelte');
  return {
    LiveStore: class extends FakeLive {
      constructor() {
        super();
        holder.current = this;
      }
    },
    sessionActive: mocks.sessionActive,
  };
});
vi.mock('./lib/preferences.svelte', () => ({
  prefs: {
    bootstrap: vi.fn(),
    load: vi.fn().mockResolvedValue({ landingPage: 'overview' }),
  },
}));
vi.mock('./lib/shell/shell-state.svelte', () => ({
  shell: { startIncidentPolling: vi.fn(), stopIncidentPolling: vi.fn() },
}));
vi.mock('./lib/shell/AppShell.svelte', async () => ({
  default: (await import('./lib/shell/testing/ShellStub.test.svelte')).default,
}));
vi.mock('./lib/shell/RouteView.svelte', stub);
vi.mock('./lib/pages/Login.svelte', stub);
vi.mock('./lib/pages/Setup.svelte', stub);
vi.mock('./lib/pages/Onboarding.svelte', stub);

const { default: App } = await import('./App.svelte');

/** Opens the app at `path`; the router is a module singleton, so resync it. */
function visit(path: string) {
  history.replaceState({}, '', path);
  router.sync();
}

const session = {
  user: { id: 'u1', username: 'admin' },
  expiresAt: '2026-11-07T00:00:00Z',
  absoluteExpiresAt: '2026-11-07T00:00:00Z',
};

beforeEach(() => {
  window.scrollTo = () => {};
  mocks.setupAvailable.mockResolvedValue(false);
  mocks.sessionActive.mockResolvedValue(false);
});

afterEach(() => {
  vi.clearAllMocks();
});

describe('session expiry (UI-5)', () => {
  it('ends the session and goes to sign-in with next when the stream says so', async () => {
    visit('/resources/res_1?range=6h');
    mocks.currentSession.mockResolvedValue(session);
    render(App);
    await waitFor(() => expect(fakeLive.current?.state).toBe('connected'));
    expect(document.querySelector('main#content')).not.toBeNull();

    fakeLive.current!.state = 'unauthorized';
    flushSync();

    expect(location.pathname).toBe('/login');
    expect(new URLSearchParams(location.search).get('next')).toBe(
      '/resources/res_1?range=6h',
    );
    expect(screen.getByTestId('stub')).toBeTruthy();
    expect(fakeLive.current?.state).toBe('disconnected');
  });

  it('signs out after a 401 only when the session is really gone', async () => {
    visit('/server');
    mocks.currentSession.mockResolvedValue(session);
    render(App);
    await waitFor(() => expect(fakeLive.current?.state).toBe('connected'));

    mocks.sessionActive.mockResolvedValueOnce(true);
    dispatchEvent(new Event(sessionEndedEvent));
    await Promise.resolve();
    await Promise.resolve();
    expect(location.pathname).toBe('/server');

    dispatchEvent(new Event(sessionEndedEvent));
    await waitFor(() => expect(location.pathname).toBe('/login'));
    expect(new URLSearchParams(location.search).get('next')).toBe('/server');
  });

  it('keeps the deep link when the visit starts without a session', async () => {
    visit('/incidents/inc_9');
    mocks.currentSession.mockResolvedValue(null);
    render(App);
    await waitFor(() => expect(location.pathname).toBe('/login'));
    expect(new URLSearchParams(location.search).get('next')).toBe(
      '/incidents/inc_9',
    );
  });
});
