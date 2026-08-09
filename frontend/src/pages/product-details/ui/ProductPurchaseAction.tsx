import { Alert, Button, Stack, Text } from '@mantine/core';
import { Link } from 'react-router';

import { getQueueAttemptRoute, type QueueAttempt } from '@/entities/queue-attempt';
import { JoinQueueButton } from '@/features/join-queue';

interface ProductPurchaseActionProps {
  attempt: QueueAttempt | null | undefined;
  isAttemptError: boolean;
  isAttemptPending: boolean;
  allocatableStock: number;
  freeStock: number;
  onJoined: (attempt: QueueAttempt) => void;
  onRetryAttempt: () => void;
  productId: string;
  queueEnabled: boolean;
  userId: string | null;
  waitingBufferCapacity: number;
  waitingCount: number;
}

const attemptActionPresentation: Record<
  QueueAttempt['state'],
  { action: string; description: string; status: string }
> = {
  waiting: {
    action: 'Вернуться в очередь',
    description: 'Ваше место сохранено. Мы покажем следующий шаг, когда товар освободится.',
    status: 'Вы уже в очереди',
  },
  invited: {
    action: 'Продолжить оформление',
    description: 'Мы ненадолго сохранили товар за вами. Продолжите оформление до конца резерва.',
    status: 'Товар ждёт вас',
  },
  checkout: {
    action: 'Продолжить оформление',
    description: 'Товар останется за вами до окончания резерва.',
    status: 'Товар сохранён за вами',
  },
  purchased: {
    action: 'Посмотреть результат',
    description: 'Посмотрите итог покупки и вернитесь в каталог.',
    status: 'Покупка завершена',
  },
  invite_expired: {
    action: 'Посмотреть результат',
    description: 'Время резерва закончилось. Вы можете попробовать купить товар снова.',
    status: 'Время резерва истекло',
  },
  checkout_expired: {
    action: 'Посмотреть результат',
    description: 'Время оформления закончилось, и резерв был освобождён.',
    status: 'Время оформления истекло',
  },
  payment_failed: {
    action: 'Посмотреть результат',
    description: 'Попробуйте ещё раз или выберите другой товар.',
    status: 'Оплата не прошла',
  },
  cancelled: {
    action: 'Посмотреть результат',
    description: 'Вы можете вернуться к товару и начать снова.',
    status: 'Покупка отменена',
  },
  sold_out: {
    action: 'Посмотреть результат',
    description: 'Товар закончился. Посмотрите похожие предложения.',
    status: 'Товар закончился',
  },
};

interface NewPurchaseActionProps {
  allocatableStock: number;
  freeStock: number;
  onJoined: (attempt: QueueAttempt) => void;
  productId: string;
  queueEnabled: boolean;
  userId: string;
  waitingBufferCapacity: number;
  waitingCount: number;
}

function NewPurchaseAction({
  allocatableStock,
  freeStock,
  onJoined,
  productId,
  queueEnabled,
  userId,
  waitingBufferCapacity,
  waitingCount,
}: NewPurchaseActionProps) {
  if (!queueEnabled || allocatableStock === 0) {
    return (
      <Stack gap="xs">
        <Button disabled fullWidth size="md">
          {queueEnabled ? 'Нет в наличии' : 'Покупка недоступна'}
        </Button>
        <Text c="dimmed" size="xs">
          {queueEnabled
            ? 'Этот товар закончился. Посмотрите похожие предложения ниже.'
            : 'Сейчас этот товар нельзя заказать. Попробуйте позже.'}
        </Text>
      </Stack>
    );
  }

  if (freeStock === 0 && waitingCount >= waitingBufferCapacity) {
    return (
      <Stack gap="xs">
        <Button disabled fullWidth size="md">
          Очередь заполнена
        </Button>
        <Text c="dimmed" size="xs">
          Попробуйте позже или выберите другой товар.
        </Text>
      </Stack>
    );
  }

  return (
    <Stack gap="xs">
      <JoinQueueButton
        label={freeStock > 0 ? 'Купить' : 'Встать в очередь'}
        onJoined={onJoined}
        productId={productId}
        userId={userId}
      />
      <Text c="dimmed" size="xs">
        {freeStock > 0
          ? 'Мы ненадолго сохраним товар за вами, чтобы вы успели продолжить оформление.'
          : 'После нажатия вы встанете в очередь. Мы сохраним ваше место и покажем следующий шаг, когда подойдёт очередь.'}
      </Text>
    </Stack>
  );
}

export function ProductPurchaseAction({
  allocatableStock,
  attempt,
  freeStock,
  isAttemptError,
  isAttemptPending,
  onJoined,
  onRetryAttempt,
  productId,
  queueEnabled,
  userId,
  waitingBufferCapacity,
  waitingCount,
}: ProductPurchaseActionProps) {
  if (userId === null) {
    return (
      <Button disabled fullWidth size="md">
        Выберите аккаунт
      </Button>
    );
  }

  if (isAttemptPending) {
    return (
      <Button disabled fullWidth size="md">
        Проверяем доступность
      </Button>
    );
  }

  if (isAttemptError) {
    return (
      <Alert color="red" title="Не удалось проверить статус покупки">
        <Stack align="flex-start" gap="sm">
          Обновите статус, прежде чем начинать новую покупку.
          <Button onClick={onRetryAttempt} size="xs" variant="light">
            Проверить ещё раз
          </Button>
        </Stack>
      </Alert>
    );
  }

  if (attempt) {
    const presentation = attemptActionPresentation[attempt.state];
    const canRetry = ['cancelled', 'checkout_expired', 'invite_expired', 'payment_failed'].includes(
      attempt.state,
    );

    return (
      <Stack gap="xs">
        <div aria-live="polite">
          <Text fw={700}>{presentation.status}</Text>
          <Text c="dimmed" size="sm">
            {presentation.description}
          </Text>
        </div>
        {canRetry ? (
          <NewPurchaseAction
            allocatableStock={allocatableStock}
            freeStock={freeStock}
            onJoined={onJoined}
            productId={productId}
            queueEnabled={queueEnabled}
            userId={userId}
            waitingBufferCapacity={waitingBufferCapacity}
            waitingCount={waitingCount}
          />
        ) : (
          <Button
            component={Link}
            fullWidth
            size="md"
            to={getQueueAttemptRoute(productId, attempt.state)}
          >
            {presentation.action}
          </Button>
        )}
      </Stack>
    );
  }

  return (
    <NewPurchaseAction
      allocatableStock={allocatableStock}
      freeStock={freeStock}
      onJoined={onJoined}
      productId={productId}
      queueEnabled={queueEnabled}
      userId={userId}
      waitingBufferCapacity={waitingBufferCapacity}
      waitingCount={waitingCount}
    />
  );
}
