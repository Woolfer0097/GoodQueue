import { skipToken, useQuery } from '@tanstack/react-query';

import { getQueueAttempt } from './queue-attempt.api';
import { queueAttemptQueryKeys } from './queue-attempt.query-keys';

export const useQueueAttemptQuery = (productId: string, userId: string | null) =>
  useQuery({
    queryFn: userId === null ? skipToken : () => getQueueAttempt(productId, userId),
    queryKey: queueAttemptQueryKeys.current(productId, userId),
  });
