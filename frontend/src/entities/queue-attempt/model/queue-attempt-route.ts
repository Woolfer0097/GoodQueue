import { generatePath } from 'react-router';

import type { QueueAttemptState } from './queue-attempt.schema';

const queueAttemptRoutePatterns = {
  cancelled: '/products/:productId/result',
  checkout: '/products/:productId/checkout',
  checkout_expired: '/products/:productId/result',
  invite_expired: '/products/:productId/result',
  invited: '/products/:productId/reservation',
  payment_failed: '/products/:productId/result',
  purchased: '/products/:productId/result',
  sold_out: '/products/:productId/result',
  waiting: '/products/:productId/queue',
} as const satisfies Record<QueueAttemptState, string>;

export const getQueueAttemptRoute = (productId: string, state: QueueAttemptState) =>
  generatePath(queueAttemptRoutePatterns[state], { productId });
