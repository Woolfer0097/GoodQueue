import {
  Alert,
  Button,
  Container,
  Flex,
  Image,
  Paper,
  Skeleton,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { useCallback, useEffect } from 'react';
import { generatePath, useNavigate, useParams } from 'react-router';

import { useCurrentDemoUser } from '@/entities/demo-user';
import { formatProductPrice, PRODUCT_IMAGE_PLACEHOLDER, useProductQuery } from '@/entities/product';
import { useQueueAttemptQuery } from '@/entities/queue-attempt';
import { CheckoutButton } from '@/features/checkout';
import { getQueueAttemptRoute } from '@/features/queue-polling';
import { formatCountdown, useDeadlineCountdown } from '@/shared/lib/deadline-countdown';

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
      <Skeleton height={150} mt="md" radius="md" />
      <Skeleton height={64} mt="md" width={180} />
      <Skeleton height={40} mt="md" width={220} />
    </Stack>
  );
}

export function CheckoutPage() {
  const { productId = '' } = useParams<{ productId: string }>();
  const { userId } = useCurrentDemoUser();
  const navigate = useNavigate();
  const attemptQuery = useQueueAttemptQuery(productId, userId);
  const productQuery = useProductQuery(productId);
  const { data: attempt, isError, isPending, refetch } = attemptQuery;
  const deadline =
    attempt?.state === 'checkout' ? (attempt.deadline_at ?? attempt.expires_at) : undefined;
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

    if (attempt.state !== 'checkout') {
      void navigate(getQueueAttemptRoute(productId, attempt.state), { replace: true });
    }
  }, [attempt, isError, isPending, navigate, productId]);

  return (
    <Container py={{ base: 'xl', sm: 64 }} size="sm">
      {isPending || productQuery.isPending ? (
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
      ) : productQuery.isError ? (
        <Alert color="red" title="Не удалось загрузить товар">
          <Stack align="flex-start" gap="md">
            <Text>Проверьте соединение и попробуйте ещё раз.</Text>
            <Button onClick={() => void productQuery.refetch()} variant="light">
              Повторить
            </Button>
          </Stack>
        </Alert>
      ) : attempt?.state === 'checkout' && productQuery.data !== undefined ? (
        <Stack gap="xl">
          <Stack gap="xs">
            <Title order={1}>Ваше право на покупку подтверждено</Title>
            <Text c="dimmed" size="lg">
              Backend выдал вам персональный временный доступ к покупке. Только после этого можно
              перейти к демонстрационной имитации оплаты.
            </Text>
          </Stack>

          <Paper p="md" radius="md" withBorder>
            <Flex
              align={{ base: 'stretch', sm: 'center' }}
              direction={{ base: 'column', sm: 'row' }}
              gap="lg"
            >
              <Image
                alt={productQuery.data.title}
                fallbackSrc={PRODUCT_IMAGE_PLACEHOLDER}
                fit="cover"
                h={128}
                radius="md"
                src={productQuery.data.image_url || PRODUCT_IMAGE_PLACEHOLDER}
                w={{ base: '100%', sm: 160 }}
              />
              <Stack gap={4}>
                <Text c="dimmed" size="sm">
                  Товар
                </Text>
                <Title order={2} size="h3">
                  {productQuery.data.title}
                </Title>
                <Text fw={700} size="xl">
                  {formatProductPrice(productQuery.data.price_cents)}
                </Text>
              </Stack>
            </Flex>
          </Paper>

          <Alert color="green" title="Вы допущены к покупке">
            Это персональное право закреплено за выбранным пользователем на ограниченное время.
          </Alert>

          <Stack gap={4}>
            <Text fw={600}>Время на завершение покупки</Text>
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
                  Право действует до {formatDeadline(deadline)}.
                </Text>
              </>
            ) : (
              <Text c="dimmed">
                Backend пока не передал точный срок. Состояние обновляется автоматически.
              </Text>
            )}
          </Stack>

          <Stack align="flex-start" gap="xs">
            <Text c="dimmed" size="sm">
              Настоящая платёжная система не вызывается. Следующее действие только имитирует оплату,
              а итоговое состояние определяет backend.
            </Text>

            <CheckoutButton attempt={attempt} productId={productId} userId={userId} />
          </Stack>
        </Stack>
      ) : null}
    </Container>
  );
}
