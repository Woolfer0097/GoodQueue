const queueAttemptRootQueryKey = ['queue-attempts'] as const;

export const queueAttemptQueryKeys = {
  all: queueAttemptRootQueryKey,
  current: (productId: string, userId: string | null) =>
    [...queueAttemptRootQueryKey, 'current', productId, userId] as const,
};
