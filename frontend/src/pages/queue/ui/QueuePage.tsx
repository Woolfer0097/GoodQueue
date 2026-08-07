import { Alert, Button, Container, Group, Skeleton, Stack, Text, Title } from '@mantine/core';
import { useEffect } from 'react';
import { generatePath, useNavigate, useParams } from 'react-router';

import { useCurrentDemoUser } from '@/entities/demo-user';
import { getQueueAttemptRoute, useQueueAttemptQuery } from '@/entities/queue-attempt';
import { CancelQueueButton } from '@/features/cancel-queue';
import { formatElapsedTime, useElapsedTime } from '@/shared/lib/elapsed-time';

function QueuePageSkeleton() {
  return (
    <Stack aria-busy="true" aria-label="Загрузка очереди" gap="md" role="status">
      <Skeleton height={38} width="60%" />
      <Skeleton height={22} width="100%" />
      <Skeleton height={22} width="80%" />
      <Skeleton height={36} mt="sm" width={180} />
    </Stack>
  );
}

export function QueuePage() {
  const { productId = '' } = useParams<{ productId: string }>();
  const { userId } = useCurrentDemoUser();
  const navigate = useNavigate();
  const { data: attempt, isError, isPending, refetch } = useQueueAttemptQuery(productId, userId);
  const elapsedSeconds = useElapsedTime(
    attempt?.state === 'waiting' ? attempt.created_at : undefined,
  );

  useEffect(() => {
    if (isPending || isError || attempt === undefined) {
      return;
    }

    if (attempt === null) {
      void navigate(generatePath('/products/:productId', { productId }), {
        replace: true,
        state: { queueNotice: 'active-attempt-missing' },
      });
      return;
    }

    if (attempt.state !== 'waiting') {
      void navigate(getQueueAttemptRoute(productId, attempt.state), { replace: true });
    }
  }, [attempt, isError, isPending, navigate, productId]);

  return (
    <Container py={{ base: 'xl', sm: 64 }} size="sm">
      {isPending ? (
        <QueuePageSkeleton />
      ) : isError ? (
        <Alert color="red" title="Не удалось обновить очередь">
          <Stack align="flex-start" gap="md">
            Проверьте соединение и попробуйте ещё раз.
            <Button onClick={() => void refetch()} variant="light">
              Повторить
            </Button>
          </Stack>
        </Alert>
      ) : attempt?.state === 'waiting' ? (
        <Stack gap="lg">
          <Stack gap="xs">
            <Title order={1}>Вы в очереди</Title>
            <Text c="dimmed" size="lg">
              Мы обновляем состояние автоматически. Оставьте страницу открытой — когда товар станет
              доступен, вы сразу перейдёте к следующему шагу.
            </Text>
          </Stack>

          {(attempt.position !== undefined || attempt.total_waiting !== undefined) && (
            <Group gap="xl">
              {attempt.position !== undefined && (
                <Text fw={600}>Ваша позиция: {attempt.position}</Text>
              )}
              {attempt.total_waiting !== undefined && (
                <Text>Ожидают покупки: {attempt.total_waiting}</Text>
              )}
            </Group>
          )}

          {elapsedSeconds !== null && (
            <Text
              aria-label={`Время в очереди: ${formatElapsedTime(elapsedSeconds)}`}
              ff="monospace"
              fw={700}
              role="timer"
              size="xl"
            >
              Время в очереди: {formatElapsedTime(elapsedSeconds)}
            </Text>
          )}

          <CancelQueueButton productId={productId} userId={userId} />
        </Stack>
      ) : null}
    </Container>
  );
}
