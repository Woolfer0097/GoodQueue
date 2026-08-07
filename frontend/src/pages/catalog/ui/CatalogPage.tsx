import {
  Alert,
  AspectRatio,
  Button,
  Container,
  EmptyState,
  SimpleGrid,
  Skeleton,
  Stack,
  Title,
} from '@mantine/core';

import { ProductCard, useProductsQuery } from '@/entities/product';

const SKELETON_COUNT = 8;

const catalogGridProps = {
  cols: { base: 2, sm: 3, lg: 4 },
  spacing: { base: 'sm', sm: 'lg' },
  verticalSpacing: { base: 'xl', sm: 40 },
} as const;

function ProductCardSkeleton() {
  return (
    <Stack gap="xs">
      <AspectRatio ratio={1}>
        <Skeleton radius="md" />
      </AspectRatio>
      <Skeleton height={16} width="80%" />
      <Skeleton height={22} width="45%" />
      <Skeleton height={16} width="55%" />
      <Skeleton height={14} width="70%" />
    </Stack>
  );
}

export function CatalogPage() {
  const { data: products, isError, isPending, refetch } = useProductsQuery();

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
                <ProductCard product={product} />
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
