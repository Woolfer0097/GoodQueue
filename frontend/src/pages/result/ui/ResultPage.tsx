import { Alert, Button, Container, Flex, Paper, Skeleton, Stack, Text, Title } from '@mantine/core';
import { useEffect } from 'react';
import { generatePath, Link, useNavigate, useParams } from 'react-router';

import { useCurrentDemoUser } from '@/entities/demo-user';
import { getQueueAttemptRoute, useQueueAttemptQuery } from '@/entities/queue-attempt';
import { JoinQueueButton } from '@/features/join-queue';
import { ProductBreadcrumbs } from '@/widgets/product-breadcrumbs';
import { RelevantProducts } from '@/widgets/relevant-products';

import {
  getResultStatePresentation,
  isTerminalQueueAttemptState,
} from '../model/result-state.presentation';

function ResultPageSkeleton() {
  return (
    <Stack aria-busy="true" aria-label="Загрузка результата" gap="md" role="status">
      <Skeleton height={38} width="70%" />
      <Skeleton height={22} width="100%" />
      <Skeleton height={38} mt="sm" width={190} />
    </Stack>
  );
}

export function ResultPage() {
  const { productId = '' } = useParams<{ productId: string }>();
  const { userId } = useCurrentDemoUser();
  const navigate = useNavigate();
  const { data: attempt, isError, isPending, refetch } = useQueueAttemptQuery(productId, userId);
  const productPath = generatePath('/products/:productId', { productId });

  useEffect(() => {
    if (
      isPending ||
      isError ||
      attempt === undefined ||
      attempt === null ||
      isTerminalQueueAttemptState(attempt.state)
    ) {
      return;
    }

    void navigate(getQueueAttemptRoute(productId, attempt.state), { replace: true });
  }, [attempt, isError, isPending, navigate, productId]);

  if (isPending) {
    return (
      <Container py={{ base: 'xl', sm: 64 }} size="lg">
        <Stack gap="lg">
          <ProductBreadcrumbs currentPage="Результат" productId={productId} />
          <ResultPageSkeleton />
        </Stack>
      </Container>
    );
  }

  if (isError) {
    return (
      <Container py={{ base: 'xl', sm: 64 }} size="lg">
        <Stack gap="lg">
          <ProductBreadcrumbs currentPage="Результат" productId={productId} />
          <Alert color="red" title="Не удалось загрузить результат">
            <Stack align="flex-start" gap="md">
              <Text>Проверьте соединение и попробуйте ещё раз.</Text>
              <Button onClick={() => void refetch()} variant="light">
                Повторить
              </Button>
              <Button component={Link} to={productPath} variant="subtle">
                Вернуться к товару
              </Button>
            </Stack>
          </Alert>
        </Stack>
      </Container>
    );
  }

  if (attempt === null) {
    return (
      <Container py={{ base: 'xl', sm: 64 }} size="lg">
        <Stack gap="lg">
          <ProductBreadcrumbs currentPage="Результат" productId={productId} />
          <Stack gap="xs">
            <Title order={1}>Покупка не найдена</Title>
            <Text c="dimmed" size="lg">
              Для этого товара нет завершённой покупки.
            </Text>
          </Stack>
          <Button component={Link} to={productPath} w="fit-content">
            Вернуться к товару
          </Button>
        </Stack>
      </Container>
    );
  }

  if (attempt === undefined || !isTerminalQueueAttemptState(attempt.state)) {
    return null;
  }

  const presentation = getResultStatePresentation(attempt.state);
  const actionPath = presentation.actionTarget === 'catalog' ? '/' : productPath;

  return (
    <Container py={{ base: 'xl', sm: 64 }} size="lg">
      <Stack gap={40}>
        <ProductBreadcrumbs currentPage="Результат" productId={productId} />
        <Paper p={{ base: 'md', sm: 'xl' }} radius="md" withBorder>
          <Stack gap="lg">
            <Stack gap="xs">
              <Title order={1}>{presentation.title}</Title>
              <Text c="dimmed" size="lg">
                {presentation.description}
              </Text>
            </Stack>

            <Flex align="center" aria-label="Действия покупки" gap="sm" role="group" wrap="wrap">
              {presentation.actionTarget === 'retry' ? (
                <JoinQueueButton
                  label={presentation.actionLabel}
                  onJoined={(nextAttempt) => {
                    void navigate(getQueueAttemptRoute(productId, nextAttempt.state));
                  }}
                  productId={productId}
                  userId={userId}
                />
              ) : (
                <Button component={Link} size="md" to={actionPath}>
                  {presentation.actionLabel}
                </Button>
              )}
              {presentation.actionTarget !== 'catalog' && (
                <Button component={Link} size="md" to="/" variant="light">
                  Вернуться в каталог
                </Button>
              )}
            </Flex>
          </Stack>
        </Paper>

        {presentation.showRelevantProducts && <RelevantProducts productId={productId} />}
      </Stack>
    </Container>
  );
}
