const queueAttemptRootQueryKey = ['queue-attempts'] as const;

export const queueAttemptQueryKeys = {
  all: queueAttemptRootQueryKey,
  active: (userId: string | null) => [...queueAttemptRootQueryKey, 'active', userId] as const,
  current: (productId: string, userId: string | null) =>
    [...queueAttemptRootQueryKey, 'current', productId, userId] as const,
};
