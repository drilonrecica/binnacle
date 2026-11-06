import { describe, expect, it, vi } from 'vitest';
import { authMethods, bootstrapExternalSession, safeRedirect } from './auth';

describe('safeRedirect', () => {
  it('allows only same-origin application paths', () => {
    vi.stubGlobal('location', { origin: 'https://binnacle.test' });
    expect(safeRedirect('/resources/res_1?range=1h')).toBe(
      '/resources/res_1?range=1h',
    );
    expect(safeRedirect('https://evil.test/steal')).toBe('/headroom');
    expect(safeRedirect('//evil.test/steal')).toBe('/headroom');
    expect(safeRedirect('/login')).toBe('/headroom');
    expect(safeRedirect('/setup')).toBe('/headroom');
    expect(safeRedirect('/not-a-route')).toBe('/headroom');
    vi.unstubAllGlobals();
  });

  it('keeps deep links to every authenticated page, old names included', () => {
    vi.stubGlobal('location', { origin: 'https://binnacle.test' });
    for (const path of [
      '/headroom',
      '/server?tab=collectors',
      '/incidents/inc_1',
      '/changes',
      '/settings/access',
      '/onboarding',
      '/alerts/incidents/inc_1',
      '/host',
    ])
      expect(safeRedirect(path)).toBe(path);
    expect(safeRedirect('/')).toBe('/');
    vi.unstubAllGlobals();
  });
});

describe('external authentication', () => {
  it('discovers methods and bootstraps through same-origin requests', async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            mode: 'local_and_proxy',
            local: true,
            proxy: true,
            proxyAvailable: true,
            mfaAvailable: true,
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      )
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetch);
    await expect(authMethods()).resolves.toMatchObject({
      proxyAvailable: true,
    });
    await expect(bootstrapExternalSession()).resolves.toBeUndefined();
    expect(fetch).toHaveBeenLastCalledWith(
      '/api/v1/auth/external-session',
      expect.objectContaining({ method: 'POST', credentials: 'same-origin' }),
    );
    vi.unstubAllGlobals();
  });
});
