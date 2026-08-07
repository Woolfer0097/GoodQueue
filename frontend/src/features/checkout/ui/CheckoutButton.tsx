import { Button } from '@mantine/core';
import { notifications } from '@mantine/notifications';

import type { QueueAttempt } from '@/entities/queue-attempt';

import { getCheckoutErrorNotification } from '../model/checkout-error';
import { useCheckout } from '../model/use-checkout';

interface CheckoutButtonProps {
  attempt: QueueAttempt | null | undefined;
  productId: string;
  userId: string | null;
}

export function CheckoutButton({ attempt, productId, userId }: CheckoutButtonProps) {
  const checkoutMutation = useCheckout({ attempt, productId, userId });
  const canCheckout = userId !== null && attempt?.state === 'checkout';

  const handleCheckout = () => {
    if (!canCheckout || checkoutMutation.isPending) {
      return;
    }

    checkoutMutation.mutate(undefined, {
      onError: (error) => {
        notifications.show({
          color: 'red',
          ...getCheckoutErrorNotification(error),
        });
      },
    });
  };

  return (
    <Button
      disabled={!canCheckout || checkoutMutation.isPending}
      loading={checkoutMutation.isPending}
      onClick={handleCheckout}
      size="md"
    >
      Имитировать оплату
    </Button>
  );
}
