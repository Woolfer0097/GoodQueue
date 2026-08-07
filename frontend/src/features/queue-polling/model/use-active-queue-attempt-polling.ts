import { skipToken, useQuery } from '@tanstack/react-query';
import { useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router';

import {
  getQueueAttempt,
  getQueueAttemptRoute,
  queueAttemptQueryKeys,
  type QueueAttemptState,
} from '@/entities/queue-attempt';

export const QUEUE_ATTEMPT_POLLING_INTERVAL_MS = 1_500;

const ACTIVE_QUEUE_ATTEMPT_STATES = new Set<QueueAttemptState>(['waiting', 'invited', 'checkout']);

export const isActiveQueueAttemptState = (state: QueueAttemptState) =>
  ACTIVE_QUEUE_ATTEMPT_STATES.has(state);

interface UseActiveQueueAttemptPollingParams {
  productId: string;
  userId: string | null;
}

export const useActiveQueueAttemptPolling = ({
  productId,
  userId,
}: UseActiveQueueAttemptPollingParams) => {
  const location = useLocation();
  const navigate = useNavigate();
  const query = useQuery({
    enabled: productId.length > 0 && userId !== null,
    meta: { background: true },
    queryFn:
      productId.length === 0 || userId === null
        ? skipToken
        : () => getQueueAttempt(productId, userId),
    queryKey: queueAttemptQueryKeys.current(productId, userId),
    refetchInterval: ({ state }) =>
      state.data !== null && state.data !== undefined && isActiveQueueAttemptState(state.data.state)
        ? QUEUE_ATTEMPT_POLLING_INTERVAL_MS
        : false,
  });

  useEffect(() => {
    if (query.data === null || query.data === undefined) {
      return;
    }

    const targetRoute = getQueueAttemptRoute(productId, query.data.state);

    if (location.pathname !== targetRoute) {
      void navigate(targetRoute, { replace: true });
    }
  }, [location.pathname, navigate, productId, query.data]);

  return query;
};
