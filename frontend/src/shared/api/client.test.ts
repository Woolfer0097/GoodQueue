import { jest } from '@jest/globals';

import { ApiError } from './api-error';
import { createApiClient } from './client';

interface ResponseOptions {
  body?: string;
  contentType?: string;
  status: number;
  statusText?: string;
}

const createResponse = ({
  body = '',
  contentType,
  status,
  statusText = '',
}: ResponseOptions): Response =>
  ({
    headers: {
      get: (name: string) => (name.toLowerCase() === 'content-type' ? (contentType ?? null) : null),
    },
    ok: status >= 200 && status < 300,
    status,
    statusText,
    text: () => Promise.resolve(body),
  }) as Response;

describe('api client', () => {
  const fetchMock = jest.fn<typeof fetch>();

  beforeEach(() => {
    fetchMock.mockReset();
  });

  it('requests a backend path relative to the configured base URL', async () => {
    fetchMock.mockResolvedValue(
      createResponse({
        body: JSON.stringify({ id: 'product-id' }),
        contentType: 'application/json',
        status: 200,
      }),
    );
    const request = createApiClient('http://localhost:8080/', fetchMock);

    await expect(request<{ id: string }>('/api/v1/products/product-id')).resolves.toEqual({
      id: 'product-id',
    });
    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/api/v1/products/product-id',
      undefined,
    );
  });

  it('passes fetch options through unchanged', async () => {
    fetchMock.mockResolvedValue(createResponse({ status: 204 }));
    const request = createApiClient('http://localhost:8080', fetchMock);
    const options: RequestInit = {
      headers: { 'X-User-ID': 'user-id' },
      method: 'DELETE',
    };

    await request('/api/v1/products/product-id/queue-entry', options);

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/api/v1/products/product-id/queue-entry',
      options,
    );
  });

  it('returns undefined for an empty successful response', async () => {
    fetchMock.mockResolvedValue(createResponse({ status: 204 }));
    const request = createApiClient('http://localhost:8080', fetchMock);

    await expect(request('/api/v1/resource')).resolves.toBeUndefined();
  });

  it('throws ApiError with parsed response data for an unsuccessful response', async () => {
    const errorData = { code: 'queue_full', message: 'Queue is full' };
    fetchMock.mockResolvedValue(
      createResponse({
        body: JSON.stringify(errorData),
        contentType: 'application/json',
        status: 409,
        statusText: 'Conflict',
      }),
    );
    const request = createApiClient('http://localhost:8080', fetchMock);

    const promise = request('/api/v1/products/product-id/queue-entries');

    await expect(promise).rejects.toMatchObject({
      data: errorData,
      status: 409,
      statusText: 'Conflict',
    });
    await expect(promise).rejects.toBeInstanceOf(ApiError);
  });
});
