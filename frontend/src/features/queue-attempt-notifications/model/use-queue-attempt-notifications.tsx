import { Anchor } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { useEffect, useRef } from 'react';
import { Link, useLocation, useNavigate } from 'react-router';

import { useCurrentDemoUser } from '@/entities/demo-user';
import {
  getQueueAttempt,
  getQueueAttemptRoute,
  type QueueAttempt,
  type QueueAttemptState,
  useActiveQueueAttemptsQuery,
} from '@/entities/queue-attempt';

const terminalStates = new Set<QueueAttemptState>([
  'purchased',
  'invite_expired',
  'checkout_expired',
  'payment_failed',
  'cancelled',
  'sold_out',
] as const);

const getStatePriority = (attempt: QueueAttempt) => {
  if (attempt.state === 'invited') {
    return 3;
  }

  if (attempt.state === 'checkout') {
    return 2;
  }

  return terminalStates.has(attempt.state) ? 1 : 0;
};

const isWaitingPositionChanged = (previous: QueueAttempt, next: QueueAttempt) =>
  previous.state === 'waiting' && next.state === 'waiting' && previous.position !== next.position;

const showPositionNotification = (attempt: QueueAttempt, knownNotificationIds: Set<string>) => {
  const href = getQueueAttemptRoute(attempt.product_id, 'waiting');
  const id = `queue-attempt:${attempt.attempt_id}`;
  const notification = {
    autoClose: false,
    color: 'blue',
    id,
    message: (
      <>
        {attempt.position === undefined
          ? 'Ваше место в очереди изменилось.'
          : `Ваше место в очереди: ${attempt.position}.`}{' '}
        <Anchor component={Link} to={href}>
          Открыть
        </Anchor>
      </>
    ),
    onClose: () => knownNotificationIds.delete(id),
    title: 'Очередь обновилась',
    withCloseButton: true,
  };

  if (knownNotificationIds.has(id)) {
    notifications.update(notification);
    return;
  }

  knownNotificationIds.add(id);
  notifications.show(notification);
};

export function useQueueAttemptNotifications() {
  const { userId } = useCurrentDemoUser();
  const location = useLocation();
  const navigate = useNavigate();
  const { data: attempts, dataUpdatedAt } = useActiveQueueAttemptsQuery(userId);
  const previousAttemptsRef = useRef<Map<string, QueueAttempt> | null>(null);
  const observedUserIdRef = useRef<string | null>(null);
  const knownNotificationIdsRef = useRef(new Set<string>());
  const missingAttemptsRef = useRef(new Map<string, QueueAttempt>());
  const pollingCycleRef = useRef(0);

  useEffect(() => {
    if (observedUserIdRef.current !== userId) {
      for (const id of knownNotificationIdsRef.current) {
        notifications.hide(id);
      }
      observedUserIdRef.current = userId;
      knownNotificationIdsRef.current.clear();
      missingAttemptsRef.current.clear();
      previousAttemptsRef.current = null;
    }

    if (attempts === undefined || userId === null) {
      return;
    }

    const nextAttempts = new Map(attempts.map((attempt) => [attempt.attempt_id, attempt]));
    const previousAttempts = previousAttemptsRef.current;
    previousAttemptsRef.current = nextAttempts;

    if (previousAttempts === null) {
      return;
    }

    const pollingCycle = pollingCycleRef.current + 1;
    pollingCycleRef.current = pollingCycle;
    const stateTransitions: QueueAttempt[] = [];
    for (const [attemptId, attempt] of nextAttempts) {
      const previousAttempt = previousAttempts.get(attemptId);
      if (!previousAttempt) {
        continue;
      }

      if (isWaitingPositionChanged(previousAttempt, attempt)) {
        showPositionNotification(attempt, knownNotificationIdsRef.current);
      }

      if (previousAttempt.state !== attempt.state) {
        stateTransitions.push(attempt);
      }
    }

    for (const [attemptId, previousAttempt] of previousAttempts) {
      if (nextAttempts.has(attemptId)) {
        missingAttemptsRef.current.delete(attemptId);
        continue;
      }

      missingAttemptsRef.current.set(attemptId, previousAttempt);
    }

    const nextStateAttempt = stateTransitions.reduce<QueueAttempt | null>((candidate, attempt) => {
      if (candidate === null || getStatePriority(attempt) > getStatePriority(candidate)) {
        return attempt;
      }

      return candidate;
    }, null);
    if (nextStateAttempt) {
      const targetRoute = getQueueAttemptRoute(nextStateAttempt.product_id, nextStateAttempt.state);
      if (location.pathname !== targetRoute) {
        void navigate(targetRoute, { replace: true });
      }
    }

    const activeStatePriority = nextStateAttempt ? getStatePriority(nextStateAttempt) : 0;
    for (const [attemptId, attempt] of missingAttemptsRef.current) {
      void getQueueAttempt(attempt.product_id, userId)
        .then((nextAttempt) => {
          if (
            pollingCycleRef.current !== pollingCycle ||
            observedUserIdRef.current !== userId ||
            missingAttemptsRef.current.get(attemptId) !== attempt ||
            nextAttempt === null ||
            nextAttempt.attempt_id !== attempt.attempt_id ||
            nextAttempt.state === attempt.state ||
            !terminalStates.has(nextAttempt.state)
          ) {
            return;
          }

          missingAttemptsRef.current.delete(attemptId);
          if (activeStatePriority > getStatePriority(nextAttempt)) {
            return;
          }

          const targetRoute = getQueueAttemptRoute(nextAttempt.product_id, nextAttempt.state);
          if (location.pathname !== targetRoute) {
            void navigate(targetRoute, { replace: true });
          }
        })
        .catch(() => undefined);
    }
  }, [attempts, dataUpdatedAt, location.pathname, navigate, userId]);
}
