import { generatePath } from 'react-router';

import type { QueueAttemptState } from './queue-attempt.schema';

const queueAttemptRoutePatterns = {
  checkout: '/products/:productId/checkout',
  invited: '/products/:productId/reservation',
  terminal: '/products/:productId/result',
  waiting: '/products/:productId/queue',
} as const;

export const getQueueAttemptRoute = (productId: string, state: QueueAttemptState) => {
  const routePattern =
    state === 'waiting'
      ? queueAttemptRoutePatterns.waiting
      : state === 'invited'
        ? queueAttemptRoutePatterns.invited
        : state === 'checkout'
          ? queueAttemptRoutePatterns.checkout
          : queueAttemptRoutePatterns.terminal;

  return generatePath(routePattern, { productId });
};
