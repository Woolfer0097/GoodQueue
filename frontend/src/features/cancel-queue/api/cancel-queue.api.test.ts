import { jest } from '@jest/globals';

const apiClientMock = jest.fn<(path: string, init?: RequestInit) => Promise<unknown>>();

jest.unstable_mockModule('@/shared/api', () => ({
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
});
