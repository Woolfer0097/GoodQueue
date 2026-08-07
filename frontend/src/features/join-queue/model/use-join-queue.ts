import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useRef } from 'react';

import { queueAttemptQueryKeys } from '@/entities/queue-attempt';

import { joinQueue } from '../api/join-queue.api';
import { createJoinQueueIdempotencyKey } from './create-join-queue-idempotency-key';
import { shouldReuseJoinQueueIdempotencyKey } from './join-queue-error';

interface UseJoinQueueParams {
  productId: string;
  userId: string | null;
}

export const useJoinQueue = ({ productId, userId }: UseJoinQueueParams) => {
  const queryClient = useQueryClient();
  const idempotencyKeyRef = useRef<string | null>(null);

  return useMutation({
    mutationFn: () => {
      if (userId === null) {
        return Promise.reject(new Error('Demo user is not selected'));
      }

      idempotencyKeyRef.current ??= createJoinQueueIdempotencyKey();

      return joinQueue(productId, userId, idempotencyKeyRef.current);
    },
    onError: (error) => {
      if (!shouldReuseJoinQueueIdempotencyKey(error)) {
        idempotencyKeyRef.current = null;
      }
    },
    onSuccess: async (attempt) => {
      await queryClient.invalidateQueries({
        queryKey: queueAttemptQueryKeys.all,
        refetchType: 'none',
      });
      queryClient.setQueryData(queueAttemptQueryKeys.current(productId, userId), attempt);
      idempotencyKeyRef.current = null;
    },
  });
};
