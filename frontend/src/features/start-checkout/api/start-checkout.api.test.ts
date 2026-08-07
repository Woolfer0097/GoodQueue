import { jest } from '@jest/globals';

const apiClientMock = jest.fn<(path: string, init?: RequestInit) => Promise<unknown>>();

jest.unstable_mockModule('@/shared/api', () => ({
  ApiError: class extends Error {},
  apiClient: apiClientMock,
}));

const { startCheckout } = await import('./start-checkout.api');

const checkoutAttempt = {
  attempt_id: '22222222-2222-4222-8222-222222222222',
  checkout_started_at: '2026-08-07T10:01:00Z',
  created_at: '2026-08-07T10:00:00Z',
  deadline_at: '2026-08-07T10:06:00Z',
  message_code: 'checkout_started',
  next_action: 'complete_payment',
  product_id: '11111111-1111-4111-8111-111111111111',
  queue_sequence: 1,
  state: 'checkout',
  updated_at: '2026-08-07T10:01:00Z',
};

describe('start checkout API', () => {
  beforeEach(() => {
    apiClientMock.mockReset();
    apiClientMock.mockResolvedValue(checkoutAttempt);
  });

  it('uses the actual Swagger checkout operation and validates its attempt response', async () => {
    const attemptId = checkoutAttempt.attempt_id;
    const userId = '00000000-0000-4000-8000-000000000001';

    await expect(startCheckout(attemptId, userId)).resolves.toEqual(checkoutAttempt);
    expect(apiClientMock).toHaveBeenCalledWith(`/api/v1/queue-attempts/${attemptId}/checkout`, {
      headers: { 'X-User-ID': userId },
      method: 'POST',
    });
  });

  it('rejects a response that is not a valid backend attempt', async () => {
    apiClientMock.mockResolvedValue({ ...checkoutAttempt, state: 'made_up_state' });

    await expect(
      startCheckout(checkoutAttempt.attempt_id, '00000000-0000-4000-8000-000000000001'),
    ).rejects.toThrow();
  });
});
