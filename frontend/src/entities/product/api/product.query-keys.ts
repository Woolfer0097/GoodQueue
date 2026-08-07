const productRootQueryKey = ['products'] as const;

export const productQueryKeys = {
  all: productRootQueryKey,
  detail: (productId: string) => [...productRootQueryKey, 'detail', productId] as const,
  list: () => [...productRootQueryKey, 'list'] as const,
};
