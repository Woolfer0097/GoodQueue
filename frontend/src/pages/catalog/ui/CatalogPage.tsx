import { Alert, Button, Container, EmptyState, SimpleGrid, Stack, Title } from '@mantine/core';

import { useCurrentDemoUser } from '@/entities/demo-user';
import {
  ProductCard,
  ProductCardSkeleton,
  type ProductCardUserStatus,
  useProductsQuery,
} from '@/entities/product';
import {
  getQueueAttemptRoute,
  type QueueAttempt,
  useActiveQueueAttemptsQuery,
} from '@/entities/queue-attempt';

const SKELETON_COUNT = 8;

const catalogGridProps = {
  cols: { base: 2, sm: 3, lg: 4 },
  spacing: { base: 'sm', sm: 'lg' },
  verticalSpacing: { base: 'xl', sm: 40 },
} as const;

const getProductUserStatus = (attempt: QueueAttempt): ProductCardUserStatus | undefined => {
  const href = getQueueAttemptRoute(attempt.product_id, attempt.state);

  switch (attempt.state) {
    case 'waiting':
      return {
        href,
        label: attempt.position ? `Вы в очереди · место ${attempt.position}` : 'Вы в очереди',
        tone: 'waiting',
      };
    case 'invited':
      return { href, label: 'Покупка доступна', tone: 'ready' };
    case 'checkout':
      return { href, label: 'Оформление начато', tone: 'checkout' };
    case 'purchased':
    case 'invite_expired':
    case 'checkout_expired':
    case 'payment_failed':
    case 'cancelled':
    case 'sold_out':
      return undefined;
  }
};

export function CatalogPage() {
  const { data: products, isError, isPending, refetch } = useProductsQuery();
  const { userId } = useCurrentDemoUser();
  const { data: activeQueueAttempts } = useActiveQueueAttemptsQuery(userId);
  const activeQueueStatusByProduct = new Map(
    activeQueueAttempts?.map((attempt) => [attempt.product_id, getProductUserStatus(attempt)]),
  );

  return (
    <Container size="xl" py={{ base: 'md', sm: 'xl' }}>
      <Stack gap="xl">
        <Title order={1}>Каталог товаров</Title>

        {isPending ? (
          <SimpleGrid
            {...catalogGridProps}
            aria-busy="true"
            aria-label="Загрузка товаров"
            role="status"
          >
            {Array.from({ length: SKELETON_COUNT }, (_, index) => (
              <ProductCardSkeleton key={index} />
            ))}
          </SimpleGrid>
        ) : isError && !products ? (
          <Alert color="red" title="Не удалось загрузить товары">
            <Stack align="flex-start" gap="md">
              Проверьте соединение и попробуйте ещё раз.
              <Button onClick={() => void refetch()} variant="light">
                Повторить
              </Button>
            </Stack>
          </Alert>
        ) : products?.length ? (
          <SimpleGrid {...catalogGridProps} role="list">
            {products.map((product) => (
              <div key={product.id} role="listitem">
                <ProductCard
                  product={product}
                  userStatus={activeQueueStatusByProduct.get(product.id)}
                />
              </div>
            ))}
          </SimpleGrid>
        ) : (
          <EmptyState
            description="Загляните позже — каталог скоро обновится."
            size="md"
            title="Товаров пока нет"
          />
        )}
      </Stack>
    </Container>
  );
}
