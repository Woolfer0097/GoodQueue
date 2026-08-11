import { jest } from '@jest/globals';
import { ZodError } from 'zod';

class ApiErrorMock extends Error {
  readonly data: unknown;
  readonly status: number;
  readonly statusText: string;

  constructor(status: number, data: unknown = undefined) {
    super(`HTTP request failed: ${status}`);
    this.data = data;
    this.status = status;
    this.statusText = '';
  }
}

const apiClientMock = jest.fn<(path: string, init?: RequestInit) => Promise<unknown>>();

jest.unstable_mockModule('@/shared/api', () => ({
  ApiError: ApiErrorMock,
  apiClient: apiClientMock,
}));

const { getActiveQueueAttempts, getQueueAttempt } = await import('./queue-attempt.api');

const productId = '11111111-1111-1111-1111-111111111111';
const userId = '00000000-0000-4000-8000-000000000002';
const queueAttemptResponse = {
  attempt_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
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

describe('queue attempt API', () => {
  beforeEach(() => {
    apiClientMock.mockReset();
  });

  it('requests and validates the current user attempt for a product', async () => {
    apiClientMock.mockResolvedValue(queueAttemptResponse);

    await expect(getQueueAttempt(productId, userId)).resolves.toEqual(queueAttemptResponse);
    expect(apiClientMock).toHaveBeenCalledWith(`/api/v1/products/${productId}/queue-entry`, {
      headers: { 'X-User-ID': userId },
    });
  });

  it('requests all active attempts for the selected user in one call', async () => {
    apiClientMock.mockResolvedValue([queueAttemptResponse]);

    await expect(getActiveQueueAttempts(userId)).resolves.toEqual([queueAttemptResponse]);
    expect(apiClientMock).toHaveBeenCalledWith('/api/v1/queue-entries/active', {
      headers: { 'X-User-ID': userId },
    });
  });

  it('treats a missing attempt as normal state', async () => {
    apiClientMock.mockRejectedValue(
      new ApiErrorMock(404, { error: { code: 'not_found', message: 'resource not found' } }),
    );

    await expect(getQueueAttempt(productId, userId)).resolves.toBeNull();
  });

  it('preserves non-404 backend errors', async () => {
    const error = new ApiErrorMock(500);
    apiClientMock.mockRejectedValue(error);

    await expect(getQueueAttempt(productId, userId)).rejects.toBe(error);
  });

  it('rejects an invalid response before it reaches consumers', async () => {
    apiClientMock.mockResolvedValue({ ...queueAttemptResponse, state: 'queue_disabled' });

    await expect(getQueueAttempt(productId, userId)).rejects.toBeInstanceOf(ZodError);
  });
});
