import { useMutation, useQueryClient } from '@tanstack/react-query';

import { queueAttemptQueryKeys } from '@/entities/queue-attempt';

import { cancelQueueAttempt } from '../api/cancel-queue.api';

export const useCancelQueueAttempt = (productId: string, userId: string | null) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => {
      if (userId === null) {
        return Promise.reject(new Error('Demo user is not selected'));
      }

      return cancelQueueAttempt(productId, userId);
    },
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: queueAttemptQueryKeys.current(productId, userId),
      }),
  });
};
