import { useQuery } from '@tanstack/react-query';

import { getProduct, getProductAlternatives, getProducts } from './product.api';
import { productQueryKeys } from './product.query-keys';

const PRODUCT_STALE_TIME = 10_000;

export const useProductsQuery = () =>
  useQuery({
    queryFn: getProducts,
    queryKey: productQueryKeys.list(),
    staleTime: PRODUCT_STALE_TIME,
  });

export const useProductQuery = (productId: string) =>
  useQuery({
    queryFn: () => getProduct(productId),
    queryKey: productQueryKeys.detail(productId),
    staleTime: PRODUCT_STALE_TIME,
  });

export const useProductAlternativesQuery = (productId: string, enabled = true) =>
  useQuery({
    enabled: Boolean(productId) && enabled,
    queryFn: () => getProductAlternatives(productId),
    queryKey: productQueryKeys.alternatives(productId),
    staleTime: PRODUCT_STALE_TIME,
  });
