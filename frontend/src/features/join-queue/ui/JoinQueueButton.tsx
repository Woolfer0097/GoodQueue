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

  const handleJoin = async () => {
    if (joinMutation.isPending) {
      return;
    }

    try {
      const attempt = await joinMutation.mutateAsync();
      onJoined(attempt);
    } catch (error) {
      notifications.show({
        color: 'red',
        ...getJoinQueueErrorNotification(error),
      });
    }
  };

  return (
    <Button
      disabled={userId === null || joinMutation.isPending}
      loading={joinMutation.isPending}
      onClick={() => void handleJoin()}
      size="md"
    >
      {label}
    </Button>
  );
}
