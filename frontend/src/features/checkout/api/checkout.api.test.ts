import { jest } from '@jest/globals';
import { ZodError } from 'zod';

const apiClientMock = jest.fn<(path: string, init?: RequestInit) => Promise<unknown>>();

jest.unstable_mockModule('@/shared/api', () => ({
  ApiError: class extends Error {},
  apiClient: apiClientMock,
}));

const { checkout } = await import('./checkout.api');

const attemptId = '22222222-2222-4222-8222-222222222222';
const productId = '11111111-1111-4111-8111-111111111111';
const userId = '00000000-0000-4000-8000-000000000001';
const checkoutResponse = {
  attempt_id: attemptId,
  checkout_started_at: '2026-08-07T10:01:00Z',
  created_at: '2026-08-07T10:00:00Z',
  deadline_at: '2026-08-07T10:06:00Z',
  message_code: 'checkout_started',
  next_action: 'complete_payment',
  product_id: productId,
  queue_sequence: 1,
  state: 'checkout',
  updated_at: '2026-08-07T10:01:00Z',
};

describe('checkout API', () => {
  beforeEach(() => {
    apiClientMock.mockReset();
    apiClientMock.mockResolvedValue(checkoutResponse);
  });

  it('posts the actual attempt ID with the required user header', async () => {
    await expect(checkout(attemptId, userId)).resolves.toEqual(checkoutResponse);

    expect(apiClientMock).toHaveBeenCalledWith(`/api/v1/queue-attempts/${attemptId}/checkout`, {
      headers: { 'X-User-ID': userId },
      method: 'POST',
    });
  });

  it('rejects an invalid backend response through Zod', async () => {
    apiClientMock.mockResolvedValue({ ...checkoutResponse, state: 'payment_succeeded_locally' });

    await expect(checkout(attemptId, userId)).rejects.toBeInstanceOf(ZodError);
  });
});
