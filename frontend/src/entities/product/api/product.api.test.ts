import { jest } from '@jest/globals';
import { ZodError } from 'zod';

const apiClientMock = jest.fn<(path: string) => Promise<unknown>>();

jest.unstable_mockModule('@/shared/api', () => ({
  apiClient: apiClientMock,
}));

const { getProduct, getProductAlternatives, getProducts } = await import('./product.api');

const productResponse = {
  allocatable_stock: 3,
  category: 'collectibles',
  description: 'Коллекционный товар',
  free_stock: 2,
  id: '11111111-1111-1111-1111-111111111111',
  image_url: 'https://example.com/product.jpg',
  price_cents: 1499000,
  queue_enabled: true,
  reserved: 1,
  title: 'Лимитированный товар',
  waiting_buffer_capacity: 3,
  waiting_count: 2,
};

describe('product API', () => {
  beforeEach(() => {
    apiClientMock.mockReset();
  });

  it('requests and validates the product list', async () => {
    apiClientMock.mockResolvedValue([productResponse]);

    await expect(getProducts()).resolves.toEqual([productResponse]);
    expect(apiClientMock).toHaveBeenCalledWith('/api/v1/products');
  });

  it('requests and validates a product by ID', async () => {
    apiClientMock.mockResolvedValue(productResponse);

    await expect(getProduct(productResponse.id)).resolves.toEqual(productResponse);
    expect(apiClientMock).toHaveBeenCalledWith(`/api/v1/products/${productResponse.id}`);
  });

  it('rejects an invalid list response before it reaches UI', async () => {
    apiClientMock.mockResolvedValue([{ ...productResponse, free_stock: -1 }]);

    await expect(getProducts()).rejects.toBeInstanceOf(ZodError);
  });

  it('rejects an invalid product response before it reaches UI', async () => {
    apiClientMock.mockResolvedValue({ ...productResponse, waiting_count: '2' });

    await expect(getProduct(productResponse.id)).rejects.toBeInstanceOf(ZodError);
  });

  describe('getProductAlternatives', () => {
    const recommendationResponse = {
      ...productResponse,
      id: '22222222-2222-2222-2222-222222222222',
      reason_code: 'semantically_similar',
      recommendation_mode: 'ai_semantic',
      recommendation_score: 0.93,
      title: 'Похожий товар',
    };

    it('requests and validates product alternatives', async () => {
      apiClientMock.mockResolvedValue([recommendationResponse]);

      await expect(getProductAlternatives(productResponse.id)).resolves.toEqual([
        recommendationResponse,
      ]);
      expect(apiClientMock).toHaveBeenCalledWith(
        `/api/v1/products/${productResponse.id}/alternatives`,
      );
    });

    it('rejects invalid recommendation metadata', async () => {
      apiClientMock.mockResolvedValue([{ ...recommendationResponse, recommendation_score: 1.1 }]);

      await expect(getProductAlternatives(productResponse.id)).rejects.toBeInstanceOf(ZodError);
    });

    it('returns an empty alternatives list', async () => {
      apiClientMock.mockResolvedValue([]);

      await expect(getProductAlternatives(productResponse.id)).resolves.toEqual([]);
    });

    it('preserves request errors', async () => {
      const requestError = new Error('Backend unavailable');
      apiClientMock.mockRejectedValue(requestError);

      await expect(getProductAlternatives(productResponse.id)).rejects.toBe(requestError);
    });
  });
});
