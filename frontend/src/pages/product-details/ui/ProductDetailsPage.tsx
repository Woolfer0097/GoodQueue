import {
  ActionIcon,
  Alert,
  AspectRatio,
  Button,
  Container,
  EmptyState,
  Grid,
  Group,
  Image,
  Skeleton,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { useState } from 'react';
import { Link, useParams } from 'react-router';

import {
  formatProductPrice,
  type Product,
  PRODUCT_IMAGE_PLACEHOLDER,
  ProductAvailabilityBadge,
  useProductQuery,
} from '@/entities/product';

const isNotFoundError = (error: unknown) =>
  typeof error === 'object' && error !== null && 'status' in error && error.status === 404;

function CatalogLink() {
  return (
    <ActionIcon
      aria-label="Вернуться в каталог"
      color="avitoBlue"
      component={Link}
      radius="xl"
      size={48}
      to="/"
      variant="light"
    >
      <svg aria-hidden="true" fill="none" height="22" viewBox="0 0 24 24" width="22">
        <path
          d="M19 12H5m6-6-6 6 6 6"
          stroke="currentColor"
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth="2"
        />
      </svg>
    </ActionIcon>
  );
}

function ProductDetails({ product }: { product: Product }) {
  const imageSource = product.image_url || PRODUCT_IMAGE_PLACEHOLDER;
  const [isImageLoading, setIsImageLoading] = useState(Boolean(product.image_url));

  return (
    <Grid align="flex-start" gap={{ base: 'lg', md: 48 }}>
      <Grid.Col span={{ base: 12, md: 7 }}>
        <AspectRatio ratio={4 / 3}>
          <Skeleton h="100%" radius="lg" visible={isImageLoading}>
            <Image
              alt={product.title}
              fallbackSrc={PRODUCT_IMAGE_PLACEHOLDER}
              fit="cover"
              h="100%"
              onError={() => setIsImageLoading(false)}
              onLoad={() => setIsImageLoading(false)}
              radius="lg"
              src={imageSource}
              w="100%"
            />
          </Skeleton>
        </AspectRatio>
      </Grid.Col>

      <Grid.Col span={{ base: 12, md: 5 }}>
        <Stack gap="sm">
          <Text component="div" fw={700} fz={{ base: 26, sm: 34 }} lh={1.15}>
            {formatProductPrice(product.price_cents)}
          </Text>
          <Title order={1} size="h2">
            {product.title}
          </Title>
          <ProductAvailabilityBadge product={product} size="lg" variant="light" w="fit-content" />
          {(product.free_stock > 0 || product.waiting_count > 0) && (
            <Group gap="xl" mt="xs">
              {product.free_stock > 0 && (
                <Text component="div" size="sm">
                  В наличии: {product.free_stock}
                </Text>
              )}
              {product.waiting_count > 0 && (
                <Text component="div" size="sm">
                  В очереди: {product.waiting_count}
                </Text>
              )}
            </Group>
          )}
        </Stack>
      </Grid.Col>
    </Grid>
  );
}

function ProductDetailsSkeleton() {
  return (
    <Grid
      align="flex-start"
      aria-busy="true"
      aria-label="Загрузка товара"
      gap={{ base: 'lg', md: 48 }}
      role="status"
    >
      <Grid.Col span={{ base: 12, md: 7 }}>
        <AspectRatio ratio={4 / 3}>
          <Skeleton radius="lg" />
        </AspectRatio>
      </Grid.Col>
      <Grid.Col span={{ base: 12, md: 5 }}>
        <Stack gap="sm">
          <Skeleton height={36} width="45%" />
          <Skeleton height={32} width="80%" />
          <Skeleton height={26} width="35%" />
          <Skeleton height={20} width="60%" />
        </Stack>
      </Grid.Col>
    </Grid>
  );
}

export function ProductDetailsPage() {
  const { productId = '' } = useParams<{ productId: string }>();
  const { data: product, error, isError, isPending, refetch } = useProductQuery(productId);

  return (
    <Container size="xl" py={{ base: 'md', sm: 'xl' }}>
      <Stack gap="lg">
        <CatalogLink />

        {isPending ? (
          <ProductDetailsSkeleton />
        ) : isError && isNotFoundError(error) ? (
          <EmptyState size="md">
            <EmptyState.Title order={1}>Товар не найден</EmptyState.Title>
            <EmptyState.Description>
              Возможно, товар был удалён или ссылка устарела.
            </EmptyState.Description>
            <EmptyState.Actions>
              <Button component={Link} to="/" variant="light">
                Перейти в каталог
              </Button>
            </EmptyState.Actions>
          </EmptyState>
        ) : isError || !product ? (
          <Alert color="red" title="Не удалось загрузить товар">
            <Stack align="flex-start" gap="md">
              Проверьте соединение и попробуйте ещё раз.
              <Button onClick={() => void refetch()} variant="light">
                Повторить
              </Button>
            </Stack>
          </Alert>
        ) : (
          <ProductDetails product={product} />
        )}
      </Stack>
    </Container>
  );
}
