import { useMutation, useQueryClient } from '@tanstack/react-query';

import { getQueueAttempt, queueAttemptQueryKeys } from '@/entities/queue-attempt';

import { cancelQueueAttempt } from '../api/cancel-queue.api';

export const useCancelQueueAttempt = (productId: string, userId: string | null) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      if (userId === null) {
        throw new Error('Demo user is not selected');
      }

      const returnedAttempt = await cancelQueueAttempt(productId, userId);

      if (returnedAttempt !== undefined) {
        return returnedAttempt;
      }

      const refreshedAttempt = await getQueueAttempt(productId, userId);

      if (refreshedAttempt === null) {
        throw new Error('Cancelled queue attempt is missing');
      }

      return refreshedAttempt;
    },
    onSuccess: async (attempt) => {
      await queryClient.invalidateQueries({
        queryKey: queueAttemptQueryKeys.all,
        refetchType: 'none',
      });
      queryClient.setQueryData(queueAttemptQueryKeys.current(productId, userId), attempt);
    },
  });
};
