import { Button, type ButtonProps } from '@mantine/core';
import { notifications } from '@mantine/notifications';

import { useCancelQueueAttempt } from '../model/use-cancel-queue-attempt';

interface CancelQueueButtonProps {
  errorTitle?: string;
  fullWidth?: boolean;
  label?: string;
  productId: string;
  size?: ButtonProps['size'];
  userId: string | null;
}

export function CancelQueueButton({
  errorTitle = 'Не удалось выйти из очереди',
  fullWidth = false,
  label = 'Выйти из очереди',
  productId,
  size = 'sm',
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
      size={size}
      variant="light"
      w={fullWidth ? '100%' : 'fit-content'}
    >
      {label}
    </Button>
  );
}
