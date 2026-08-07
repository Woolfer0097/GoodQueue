import { jest } from '@jest/globals';
import { ZodError } from 'zod';

const apiClientMock = jest.fn<(path: string) => Promise<unknown>>();

jest.unstable_mockModule('@/shared/api', () => ({
  apiClient: apiClientMock,
}));

const { getProduct, getProducts } = await import('./product.api');

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
});
