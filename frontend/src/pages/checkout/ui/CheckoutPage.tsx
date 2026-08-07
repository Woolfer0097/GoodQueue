import { Alert, Button, Container, Skeleton, Stack, Text, Title } from '@mantine/core';
import { useEffect } from 'react';
import { generatePath, useNavigate, useParams } from 'react-router';

import { useCurrentDemoUser } from '@/entities/demo-user';
import { useQueueAttemptQuery } from '@/entities/queue-attempt';
import { CheckoutButton } from '@/features/checkout';
import { getQueueAttemptRoute } from '@/features/queue-polling';

const formatDeadline = (deadline: string) =>
  new Intl.DateTimeFormat('ru-RU', {
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    month: 'long',
  }).format(new Date(deadline));

function CheckoutPageSkeleton() {
  return (
    <Stack aria-busy="true" aria-label="Загрузка оформления" gap="md" role="status">
      <Skeleton height={38} width="75%" />
      <Skeleton height={22} width="100%" />
      <Skeleton height={40} mt="md" width={220} />
    </Stack>
  );
}

export function CheckoutPage() {
  const { productId = '' } = useParams<{ productId: string }>();
  const { userId } = useCurrentDemoUser();
  const navigate = useNavigate();
  const { data: attempt, isError, isPending, refetch } = useQueueAttemptQuery(productId, userId);
  const deadline =
    attempt?.state === 'checkout' ? (attempt.deadline_at ?? attempt.expires_at) : undefined;

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

    if (attempt.state !== 'checkout') {
      void navigate(getQueueAttemptRoute(productId, attempt.state), { replace: true });
    }
  }, [attempt, isError, isPending, navigate, productId]);

  return (
    <Container py={{ base: 'xl', sm: 64 }} size="sm">
      {isPending ? (
        <CheckoutPageSkeleton />
      ) : isError ? (
        <Alert color="red" title="Не удалось загрузить оформление">
          <Stack align="flex-start" gap="md">
            <Text>Проверьте соединение и попробуйте ещё раз.</Text>
            <Button onClick={() => void refetch()} variant="light">
              Повторить
            </Button>
          </Stack>
        </Alert>
      ) : attempt?.state === 'checkout' ? (
        <Stack gap="xl">
          <Stack gap="xs">
            <Title order={1}>Ваше право на покупку подтверждено</Title>
            <Text c="dimmed" size="lg">
              Backend выдал вам персональный временный доступ к покупке. Только после этого можно
              перейти к демонстрационной имитации оплаты.
            </Text>
          </Stack>

          <Stack gap="xs">
            {deadline !== undefined && (
              <Text fw={600}>Завершите действие до {formatDeadline(deadline)}.</Text>
            )}
            <Text c="dimmed" size="sm">
              Настоящая платёжная система не вызывается. Итоговое состояние всегда определяет
              backend.
            </Text>
          </Stack>

          <CheckoutButton attempt={attempt} productId={productId} userId={userId} />
        </Stack>
      ) : null}
    </Container>
  );
}
