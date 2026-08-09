import {
  Alert,
  Box,
  Button,
  Container,
  Flex,
  Paper,
  Skeleton,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { useCallback, useEffect } from 'react';
import { generatePath, useNavigate, useParams } from 'react-router';

import { useCurrentDemoUser } from '@/entities/demo-user';
import { useProductQuery } from '@/entities/product';
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
  const productQuery = useProductQuery(productId);
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
        <ProductBreadcrumbs
          currentPage="Резерв"
          productId={productId}
          productTitle={productQuery.data?.title}
        />
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
              <Title order={1}>Товар ждёт вас</Title>
              <Text c="dimmed" size="lg">
                Мы сохранили его за вами. Продолжите оформление, пока действует резерв.
              </Text>
            </Stack>

            <Paper p="md" radius="md" withBorder>
              <Stack gap={4}>
                <Text fw={600}>Осталось времени</Text>
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
                      Резерв действует до {formatDeadline(deadline)}.
                    </Text>
                  </>
                ) : (
                  <Text c="dimmed">
                    Не удалось показать точное время. Мы продолжаем проверять резерв.
                  </Text>
                )}
              </Stack>
            </Paper>

            <Stack gap="xs">
              <Text fw={600}>Продолжите оформление до окончания резерва.</Text>
              <Flex aria-label="Действия резерва" gap="sm" role="group" wrap="wrap">
                <Box style={{ flex: '1 1 15rem' }}>
                  <StartCheckoutButton
                    attemptId={attempt.attempt_id}
                    productId={productId}
                    userId={userId}
                  />
                </Box>
                <Box style={{ flex: '1 1 15rem' }}>
                  <CancelQueueButton
                    errorTitle="Не удалось отказаться от резерва"
                    fullWidth
                    label="Отказаться от резерва"
                    productId={productId}
                    size="md"
                    userId={userId}
                  />
                </Box>
              </Flex>
            </Stack>
          </Stack>
        ) : null}
      </Stack>
    </Container>
  );
}
