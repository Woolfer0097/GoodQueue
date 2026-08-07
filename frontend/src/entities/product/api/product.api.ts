import { apiClient } from '@/shared/api';

import { productListSchema, productSchema } from '../model/product.schema';

export const getProducts = async () => {
  const response = await apiClient('/api/v1/products');

  return productListSchema.parse(response);
};

export const getProduct = async (productId: string) => {
  const response = await apiClient(`/api/v1/products/${encodeURIComponent(productId)}`);

  return productSchema.parse(response);
};
