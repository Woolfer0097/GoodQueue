import { jest } from '@jest/globals';

import { productQueryKeys } from './product.query-keys';

const useQueryMock = jest.fn<(options: Record<string, unknown>) => unknown>();

jest.unstable_mockModule('@tanstack/react-query', () => ({
  useQuery: useQueryMock,
}));

jest.unstable_mockModule('./product.api', () => ({
  getProduct: jest.fn(),
  getProductAlternatives: jest.fn(),
  getProducts: jest.fn(),
}));

const { useProductAlternativesQuery } = await import('./product.queries');

describe('product query keys', () => {
  it('keeps list, product detail and alternatives caches separate and stable', () => {
    const productId = '11111111-1111-4111-8111-111111111111';

    expect(productQueryKeys.list()).toEqual(['products', 'list']);
    expect(productQueryKeys.detail(productId)).toEqual(['products', 'detail', productId]);
    expect(productQueryKeys.alternatives(productId)).toEqual([
      'products',
      'alternatives',
      productId,
    ]);
    expect(productQueryKeys.detail(productId)).toEqual(productQueryKeys.detail(productId));
    expect(productQueryKeys.alternatives(productId)).toEqual(
      productQueryKeys.alternatives(productId),
    );
  });
});

describe('product queries', () => {
  beforeEach(() => {
    useQueryMock.mockReset();
    useQueryMock.mockReturnValue({});
  });

  it('marks alternatives as non-blocking background content', () => {
    const productId = '11111111-1111-4111-8111-111111111111';

    useProductAlternativesQuery(productId);

    expect(useQueryMock).toHaveBeenCalledWith(
      expect.objectContaining({
        enabled: true,
        meta: { background: true },
        queryKey: productQueryKeys.alternatives(productId),
      }),
    );
  });
});
