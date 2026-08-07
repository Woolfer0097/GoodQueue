import { productQueryKeys } from './product.query-keys';

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
