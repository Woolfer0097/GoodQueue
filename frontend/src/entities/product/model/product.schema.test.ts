import { productListSchema, productSchema } from './product.schema';

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

describe('product schemas', () => {
  it('parses a product response from the backend contract', () => {
    expect(productSchema.parse(productResponse)).toEqual(productResponse);
  });

  it('parses a list of products', () => {
    expect(productListSchema.parse([productResponse])).toEqual([productResponse]);
  });

  it('accepts a project-local product image path', () => {
    const response = { ...productResponse, image_url: '/product-images/retro-robot.webp' };

    expect(productSchema.parse(response)).toEqual(response);
  });

  it.each([
    ['invalid UUID', { ...productResponse, id: 'product-id' }],
    ['negative price', { ...productResponse, price_cents: -1 }],
    ['fractional stock', { ...productResponse, free_stock: 1.5 }],
    ['invalid image URL', { ...productResponse, image_url: 'product.jpg' }],
  ])('rejects %s', (_caseName, response) => {
    expect(() => productSchema.parse(response)).toThrow();
  });
});
