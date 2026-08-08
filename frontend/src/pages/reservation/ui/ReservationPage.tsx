import { Alert, Button, Container, Group, Skeleton, Stack, Text, Title } from '@mantine/core';
import { useCallback, useEffect } from 'react';
import { generatePath, useNavigate, useParams } from 'react-router';

import { useCurrentDemoUser } from '@/entities/demo-user';
import { getQueueAttemptRoute, useQueueAttemptQuery } from '@/entities/queue-attempt';
import { CancelQueueButton } from '@/features/cancel-queue';
import { formatCountdown, useDeadlineCountdown } from '@/shared/lib/deadline-countdown';
import { ProductBreadcrumbs } from '@/widgets/product-breadcrumbs';

import { StartCheckoutButton } from './StartCheckoutButton';

const formatDeadline = (deadline: string) =>
  new Intl.DateTimeFormat('ru-RU', {
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    month: 'long',
  }).format(new Date(deadline));

function ReservationPageSkeleton() {
  return (
    <Stack aria-busy="true" aria-label="Загрузка резерва" gap="md" role="status">
      <Skeleton height={38} width="75%" />
      <Skeleton height={22} width="100%" />
      <Skeleton height={64} mt="md" width={180} />
      <Skeleton height={40} mt="sm" width={260} />
    </Stack>
  );
}

export function ReservationPage() {
  const { productId = '' } = useParams<{ productId: string }>();
  const { userId } = useCurrentDemoUser();
  const navigate = useNavigate();
  const { data: attempt, isError, isPending, refetch } = useQueueAttemptQuery(productId, userId);
  const deadline =
    attempt?.state === 'invited' ? (attempt.deadline_at ?? attempt.expires_at) : undefined;
  const refetchExpiredAttempt = useCallback(() => {
    void refetch();
  }, [refetch]);
  const remainingSeconds = useDeadlineCountdown(deadline, refetchExpiredAttempt);

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

    if (attempt.state !== 'invited') {
      void navigate(getQueueAttemptRoute(productId, attempt.state), { replace: true });
    }
  }, [attempt, isError, isPending, navigate, productId]);

  return (
    <Container py={{ base: 'xl', sm: 64 }} size="sm">
      <Stack gap="lg">
        <ProductBreadcrumbs currentPage="Резерв" productId={productId} />
        {isPending ? (
          <ReservationPageSkeleton />
        ) : isError ? (
          <Alert color="red" title="Не удалось обновить резерв">
            <Stack align="flex-start" gap="md">
              Проверьте соединение и попробуйте ещё раз.
              <Button onClick={() => void refetch()} variant="light">
                Повторить
              </Button>
            </Stack>
          </Alert>
        ) : attempt?.state === 'invited' ? (
          <Stack gap="xl">
            <Stack gap="xs">
              <Title order={1}>Товар зарезервирован для вас</Title>
              <Text c="dimmed" size="lg">
                Это право персональное: только вы можете воспользоваться резервом и перейти к
                оформлению.
              </Text>
            </Stack>

            <Stack gap={4}>
              <Text fw={600}>Право на покупку ограничено временем</Text>
              {deadline !== undefined && remainingSeconds !== null ? (
                <>
                  <Text
                    aria-label={`Осталось времени: ${formatCountdown(remainingSeconds)}`}
                    ff="monospace"
                    fw={800}
                    lh={1}
                    role="timer"
                    size="3rem"
                  >
                    {formatCountdown(remainingSeconds)}
                  </Text>
                  <Text c="dimmed" size="sm">
                    Срок резерва: {formatDeadline(deadline)}
                  </Text>
                </>
              ) : (
                <Text c="dimmed">
                  Backend пока не передал точный срок. Состояние обновляется автоматически.
                </Text>
              )}
            </Stack>

            <Stack gap="xs">
              <Text fw={600}>Следующий шаг — перейти к оформлению до окончания резерва.</Text>
              <Group align="center" gap="sm">
                <StartCheckoutButton
                  attemptId={attempt.attempt_id}
                  productId={productId}
                  userId={userId}
                />
                <CancelQueueButton productId={productId} userId={userId} />
              </Group>
            </Stack>
          </Stack>
        ) : null}
      </Stack>
    </Container>
  );
}
