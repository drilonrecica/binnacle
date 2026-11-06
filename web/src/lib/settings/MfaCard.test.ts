// @vitest-environment jsdom
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { polyfillDom } from '../ui/testing';

vi.mock('../api/access', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/access')>()),
  mfaStatus: vi.fn().mockResolvedValue({ enabled: false }),
  enrollMfa: vi.fn().mockResolvedValue({
    seed: 'JBSWY3DPEHPK3PXP',
    uri: 'otpauth://totp/Binnacle:admin?secret=JBSWY3DPEHPK3PXP',
    expiresAt: '2026-07-11T12:10:00Z',
  }),
  confirmMfa: vi.fn().mockResolvedValue({
    recoveryCodes: ['AAAA-BBBB-CCCC-DDDD', 'EEEE-FFFF-GGGG-HHHH'],
  }),
}));

const { default: MfaCard } = await import('./MfaCard.svelte');

beforeAll(polyfillDom);
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('MfaCard', () => {
  it('closes the enrolment dialog so the one-time recovery codes are reachable (DSC-31)', async () => {
    const user = userEvent.setup();
    render(MfaCard, { available: true, onsignedout: vi.fn() });
    await user.click(
      await screen.findByRole('button', { name: /Set up two-factor/ }),
    );
    const dialog = screen.getByRole('dialog');
    await user.type(
      within(dialog).getByLabelText(/Password/),
      'correct horse battery',
    );
    await user.click(within(dialog).getByRole('button', { name: 'Continue' }));
    await within(dialog).findByText('JBSWY3DPEHPK3PXP');
    await user.type(
      within(dialog).getByLabelText(/Code from your app/),
      '123456',
    );
    await user.click(within(dialog).getByRole('button', { name: 'Enable' }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(screen.getByText('AAAA-BBBB-CCCC-DDDD')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Done' }));
    expect(screen.queryByText('AAAA-BBBB-CCCC-DDDD')).toBeNull();
  });
});
