const productRootQueryKey = ['products'] as const;

export const productQueryKeys = {
  all: productRootQueryKey,
  alternatives: (productId: string) => [...productRootQueryKey, 'alternatives', productId] as const,
  detail: (productId: string) => [...productRootQueryKey, 'detail', productId] as const,
  list: () => [...productRootQueryKey, 'list'] as const,
};
