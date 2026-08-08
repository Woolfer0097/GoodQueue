import { Button } from '@mantine/core';
import { notifications } from '@mantine/notifications';

import { useCancelQueueAttempt } from '../model/use-cancel-queue-attempt';

interface CancelQueueButtonProps {
  errorTitle?: string;
  label?: string;
  productId: string;
  userId: string | null;
}

export function CancelQueueButton({
  errorTitle = 'Не удалось выйти из очереди',
  label = 'Выйти из очереди',
  productId,
  userId,
}: CancelQueueButtonProps) {
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
          title: errorTitle,
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
      size="xs"
      variant="subtle"
    >
      {label}
    </Button>
  );
}
