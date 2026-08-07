import { Button } from '@mantine/core';
import { notifications } from '@mantine/notifications';

import { useCancelQueueAttempt } from '../model/use-cancel-queue-attempt';

interface CancelQueueButtonProps {
  productId: string;
  userId: string | null;
}

export function CancelQueueButton({ productId, userId }: CancelQueueButtonProps) {
  const cancelMutation = useCancelQueueAttempt(productId, userId);

  const handleCancel = () => {
    if (cancelMutation.isPending) {
      return;
    }

    cancelMutation.mutate(undefined, {
      onError: () => {
        notifications.show({
          color: 'red',
          message: 'Проверьте соединение и попробуйте ещё раз.',
          title: 'Не удалось выйти из очереди',
        });
      },
    });
  };

  return (
    <Button
      color="red"
      disabled={userId === null || cancelMutation.isPending}
      loading={cancelMutation.isPending}
      onClick={handleCancel}
      variant="light"
    >
      Выйти из очереди
    </Button>
  );
}
