import { Button } from '@mantine/core';
import { notifications } from '@mantine/notifications';

import type { QueueAttempt } from '@/entities/queue-attempt';

import { getJoinQueueErrorNotification } from '../model/join-queue-error';
import { useJoinQueue } from '../model/use-join-queue';

interface JoinQueueButtonProps {
  label?: string;
  onJoined: (attempt: QueueAttempt) => void;
  productId: string;
  userId: string | null;
}

export function JoinQueueButton({
  label = 'Купить',
  onJoined,
  productId,
  userId,
}: JoinQueueButtonProps) {
  const joinMutation = useJoinQueue({ productId, userId });

  const handleJoin = () => {
    if (joinMutation.isPending) {
      return;
    }

    joinMutation.mutate(undefined, {
      onError: (error) => {
        notifications.show({
          color: 'red',
          ...getJoinQueueErrorNotification(error),
        });
      },
      onSuccess: (attempt) => {
        onJoined(attempt);
      },
    });
  };

  return (
    <Button
      disabled={userId === null || joinMutation.isPending}
      loading={joinMutation.isPending}
      onClick={handleJoin}
      size="md"
    >
      {label}
    </Button>
  );
}
