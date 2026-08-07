import { Outlet, useParams } from 'react-router';

import { useCurrentDemoUser } from '@/entities/demo-user';

import { useActiveQueueAttemptPolling } from '../model/use-active-queue-attempt-polling';

export function QueuePollingRoute() {
  const { productId = '' } = useParams<{ productId: string }>();
  const { userId } = useCurrentDemoUser();

  useActiveQueueAttemptPolling({ productId, userId });

  return <Outlet />;
}
