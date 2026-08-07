import { Button } from '@mantine/core';
import { notifications } from '@mantine/notifications';

import { useStartCheckout } from '../model/use-start-checkout';

interface StartCheckoutButtonProps {
  attemptId: string;
  productId: string;
  userId: string | null;
}

export function StartCheckoutButton({ attemptId, productId, userId }: StartCheckoutButtonProps) {
  const startCheckoutMutation = useStartCheckout({ attemptId, productId, userId });

  const handleStartCheckout = () => {
    startCheckoutMutation.mutate(undefined, {
      onError: () => {
        notifications.show({
          color: 'red',
          message: 'Обновите состояние резерва и попробуйте ещё раз.',
          title: 'Не удалось перейти к оформлению',
        });
      },
    });
  };

  return (
    <Button
      disabled={userId === null}
      loading={startCheckoutMutation.isPending}
      onClick={handleStartCheckout}
      size="md"
    >
      Перейти к оформлению
    </Button>
  );
}
