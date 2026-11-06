// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { router } from '../router.svelte';

const mocks = vi.hoisted(() => ({ login: vi.fn() }));

vi.mock('../auth', async () => ({
  ...(await vi.importActual<typeof import('../auth')>('../auth')),
  login: mocks.login,
  authMethods: vi.fn().mockResolvedValue({
    mode: 'local',
    local: true,
    proxy: false,
    proxyAvailable: false,
    mfaAvailable: false,
  }),
}));

const { default: Login } = await import('./Login.svelte');

function visit(path: string) {
  history.replaceState({}, '', path);
  router.sync();
}

async function signIn() {
  const user = userEvent.setup();
  await user.type(screen.getByLabelText(/Username/), 'admin');
  await user.type(screen.getByLabelText(/Password/), 'correct horse battery');
  await user.click(screen.getByRole('button', { name: 'Sign in' }));
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('Login honours next (UI-5)', () => {
  it('returns to the deep link after signing in', async () => {
    visit('/login?next=%2Fresources%2Fres_1%3Frange%3D6h');
    mocks.login.mockResolvedValue(undefined);
    const onauthenticated = vi.fn();
    render(Login, { onauthenticated });

    expect(screen.getByText('/resources/res_1?range=6h')).toBeTruthy();
    await signIn();
    await waitFor(() =>
      expect(onauthenticated).toHaveBeenCalledWith('/resources/res_1?range=6h'),
    );
  });

  it('goes home when next is missing or points off-site', async () => {
    visit('/login?next=https%3A%2F%2Fevil.example%2Fsteal');
    mocks.login.mockResolvedValue(undefined);
    const onauthenticated = vi.fn();
    render(Login, { onauthenticated });

    expect(screen.queryByText(/After signing in you return/)).toBeNull();
    await signIn();
    await waitFor(() =>
      expect(onauthenticated).toHaveBeenCalledWith('/headroom'),
    );
  });
});
