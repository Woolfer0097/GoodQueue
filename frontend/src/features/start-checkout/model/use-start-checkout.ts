import { useMutation, useQueryClient } from '@tanstack/react-query';

import { queueAttemptQueryKeys } from '@/entities/queue-attempt';

import { startCheckout } from '../api/start-checkout.api';

interface UseStartCheckoutParams {
  attemptId: string;
  productId: string;
  userId: string | null;
}

export const useStartCheckout = ({ attemptId, productId, userId }: UseStartCheckoutParams) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => {
      if (userId === null) {
        return Promise.reject(new Error('Demo user is not selected'));
      }

      return startCheckout(attemptId, userId);
    },
    onSuccess: (attempt) => {
      queryClient.setQueryData(queueAttemptQueryKeys.current(productId, userId), attempt);
    },
  });
};
