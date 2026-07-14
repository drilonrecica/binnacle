import { describe, expect, it, vi } from 'vitest';
import { authMethods, bootstrapExternalSession, safeRedirect } from './auth';

describe('safeRedirect', () => {
  it('allows only same-origin application paths', () => {
    vi.stubGlobal('location', { origin: 'https://binnacle.test' });
    expect(safeRedirect('/resources/res_1?range=1h')).toBe(
      '/resources/res_1?range=1h',
    );
    expect(safeRedirect('https://evil.test/steal')).toBe('/watch');
    expect(safeRedirect('//evil.test/steal')).toBe('/watch');
    expect(safeRedirect('/login')).toBe('/watch');
    expect(safeRedirect('/overview')).toBe('/watch');
    expect(safeRedirect('/not-a-route')).toBe('/watch');
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
