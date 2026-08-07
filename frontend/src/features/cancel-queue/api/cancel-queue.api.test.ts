import { jest } from '@jest/globals';

const apiClientMock = jest.fn<(path: string, init?: RequestInit) => Promise<unknown>>();

jest.unstable_mockModule('@/shared/api', () => ({
  ApiError: class ApiError extends Error {},
  apiClient: apiClientMock,
}));

const { cancelQueueAttempt } = await import('./cancel-queue.api');

describe('cancel queue API', () => {
  beforeEach(() => {
    apiClientMock.mockReset();
    apiClientMock.mockResolvedValue(undefined);
  });

  it('cancels the selected user attempt for the route product', async () => {
    const productId = '11111111-1111-4111-8111-111111111111';
    const userId = '00000000-0000-4000-8000-000000000002';

    await expect(cancelQueueAttempt(productId, userId)).resolves.toBeUndefined();
    expect(apiClientMock).toHaveBeenCalledWith(`/api/v1/products/${productId}/queue-entry`, {
      headers: { 'X-User-ID': userId },
      method: 'DELETE',
    });
  });

  it('uses a backend-returned attempt when the endpoint provides one', async () => {
    const cancelledAttempt = {
      attempt_id: '22222222-2222-4222-8222-222222222222',
      created_at: '2026-08-07T10:00:00Z',
      message_code: 'cancelled',
      next_action: 'join_queue',
      product_id: '11111111-1111-4111-8111-111111111111',
      queue_sequence: 2,
      state: 'cancelled',
      terminal_at: '2026-08-07T10:01:00Z',
      updated_at: '2026-08-07T10:01:00Z',
    };
    apiClientMock.mockResolvedValue(cancelledAttempt);

    await expect(
      cancelQueueAttempt(cancelledAttempt.product_id, '00000000-0000-4000-8000-000000000002'),
    ).resolves.toEqual(cancelledAttempt);
  });

  it('rejects an invalid backend-returned attempt', async () => {
    apiClientMock.mockResolvedValue({ state: 'cancelled' });

    await expect(
      cancelQueueAttempt(
        '11111111-1111-4111-8111-111111111111',
        '00000000-0000-4000-8000-000000000002',
      ),
    ).rejects.toThrow();
  });
});
