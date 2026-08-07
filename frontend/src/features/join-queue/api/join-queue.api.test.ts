import { jest } from '@jest/globals';
import { ZodError } from 'zod';

const apiClientMock = jest.fn<(path: string, init?: RequestInit) => Promise<unknown>>();

jest.unstable_mockModule('@/shared/api', () => ({
  ApiError: class ApiErrorMock extends Error {},
  apiClient: apiClientMock,
}));

const { joinQueue } = await import('./join-queue.api');

const productId = '11111111-1111-4111-8111-111111111111';
const userId = '00000000-0000-4000-8000-000000000002';
const idempotencyKey = '33333333-3333-4333-8333-333333333333';
const queueAttemptResponse = {
  attempt_id: '22222222-2222-4222-8222-222222222222',
  created_at: '2026-08-07T10:00:00Z',
  message_code: 'queue_waiting',
  next_action: 'wait',
  position: 1,
  position_ahead: 0,
  product_id: productId,
  queue_sequence: 2,
  state: 'waiting',
  total_waiting: 2,
  updated_at: '2026-08-07T10:00:01Z',
};

describe('join queue API', () => {
  beforeEach(() => {
    apiClientMock.mockReset();
  });

  it('posts the selected user and scoped idempotency key and validates the response', async () => {
    apiClientMock.mockResolvedValue(queueAttemptResponse);

    await expect(joinQueue(productId, userId, idempotencyKey)).resolves.toEqual(
      queueAttemptResponse,
    );
    expect(apiClientMock).toHaveBeenCalledWith(`/api/v1/products/${productId}/queue-entries`, {
      headers: {
        'Idempotency-Key': idempotencyKey,
        'X-User-ID': userId,
      },
      method: 'POST',
    });
  });

  it('rejects an invalid backend response', async () => {
    apiClientMock.mockResolvedValue({ ...queueAttemptResponse, state: 'queue_disabled' });

    await expect(joinQueue(productId, userId, idempotencyKey)).rejects.toBeInstanceOf(ZodError);
  });
});
