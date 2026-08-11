import { skipToken, useQuery } from '@tanstack/react-query';

import { getActiveQueueAttempts, getQueueAttempt } from './queue-attempt.api';
import { queueAttemptQueryKeys } from './queue-attempt.query-keys';

export const useQueueAttemptQuery = (productId: string, userId: string | null) =>
  useQuery({
    queryFn: userId === null ? skipToken : () => getQueueAttempt(productId, userId),
    queryKey: queueAttemptQueryKeys.current(productId, userId),
  });

export const useActiveQueueAttemptsQuery = (userId: string | null) =>
  useQuery({
    queryFn: userId === null ? skipToken : () => getActiveQueueAttempts(userId),
    queryKey: queueAttemptQueryKeys.active(userId),
    refetchInterval: userId === null ? false : 5_000,
  });
