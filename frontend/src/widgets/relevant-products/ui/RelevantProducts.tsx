import { Alert, Button, SimpleGrid, Stack, Text, Title } from '@mantine/core';

import { ProductCard, ProductCardSkeleton, useProductAlternativesQuery } from '@/entities/product';

const SKELETON_COUNT = 4;

const relevantProductsGridProps = {
  cols: { base: 2, sm: 3, lg: 4 },
  spacing: { base: 'sm', sm: 'lg' },
  verticalSpacing: { base: 'xl', sm: 40 },
} as const;

interface RelevantProductsProps {
  productId: string;
  title?: string;
}

export function RelevantProducts({ productId, title = 'Похожие товары' }: RelevantProductsProps) {
  const { data, isError, isPending, refetch } = useProductAlternativesQuery(productId);
  const products = data?.filter((product) => product.id !== productId);
  const resolvedTitle =
    title === 'Похожие товары' &&
    products?.length &&
    products.every((product) => product.reason_code === 'available_now')
      ? 'Другие доступные товары'
      : title;

  return (
    <Stack aria-label={resolvedTitle} component="section" gap="lg">
      <Title order={2}>{resolvedTitle}</Title>

      {isPending ? (
        <SimpleGrid
          {...relevantProductsGridProps}
          aria-busy="true"
          aria-label="Загрузка похожих товаров"
          role="status"
        >
          {Array.from({ length: SKELETON_COUNT }, (_, index) => (
            <ProductCardSkeleton key={index} />
          ))}
        </SimpleGrid>
      ) : products?.length ? (
        <SimpleGrid {...relevantProductsGridProps} role="list">
          {products.map((product) => (
            <div key={product.id} role="listitem">
              <ProductCard product={product} />
            </div>
          ))}
        </SimpleGrid>
      ) : isError ? (
        <Alert color="yellow" title="Не удалось загрузить похожие товары">
          <Stack align="flex-start" gap="sm">
            Основной сценарий доступен. Попробуйте загрузить рекомендации ещё раз.
            <Button onClick={() => void refetch()} size="xs" variant="light">
              Повторить
            </Button>
          </Stack>
        </Alert>
      ) : (
        <Text c="dimmed">Похожих товаров пока нет.</Text>
      )}
    </Stack>
  );
}
