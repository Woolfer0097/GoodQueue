import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router';

import {
  getQueueAttemptRoute,
  type QueueAttempt,
  queueAttemptQueryKeys,
} from '@/entities/queue-attempt';

import { checkout } from '../api/checkout.api';

interface UseCheckoutParams {
  attempt: QueueAttempt | null | undefined;
  productId: string;
  userId: string | null;
}

export const useCheckout = ({ attempt, productId, userId }: UseCheckoutParams) => {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => {
      if (userId === null || attempt?.state !== 'checkout') {
        return Promise.reject(new Error('Checkout is not available for the current attempt'));
      }

      return checkout(attempt.attempt_id, userId);
    },
    onSuccess: async (updatedAttempt) => {
      await queryClient.invalidateQueries({
        queryKey: queueAttemptQueryKeys.all,
        refetchType: 'none',
      });
      queryClient.setQueryData(queueAttemptQueryKeys.current(productId, userId), updatedAttempt);

      await navigate(getQueueAttemptRoute(productId, updatedAttempt.state));
    },
  });
};
