import {
  Alert,
  AspectRatio,
  Button,
  Container,
  EmptyState,
  Grid,
  Group,
  Image,
  Paper,
  Skeleton,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { useState } from 'react';
import { Link, useLocation, useNavigate, useParams } from 'react-router';

import {
  formatProductCategory,
  formatProductPrice,
  type Product,
  PRODUCT_IMAGE_PLACEHOLDER,
  ProductAvailabilityBadge,
  useProductQuery,
} from '@/entities/product';
import {
  getQueueAttemptRoute,
  type QueueAttempt,
  useQueueAttemptQuery,
} from '@/entities/queue-attempt';
import { useCurrentDemoUser } from '@/features/select-demo-user';
import { ProductBreadcrumbs } from '@/widgets/product-breadcrumbs';
import { RelevantProducts } from '@/widgets/relevant-products';

import { ProductPurchaseAction } from './ProductPurchaseAction';

const isNotFoundError = (error: unknown) => {
  if (typeof error !== 'object' || error === null || !('status' in error)) {
    return false;
  }

  if (error.status === 404) {
    return true;
  }

  if (error.status !== 400 || !('data' in error)) {
    return false;
  }

  const { data } = error;

  return (
    typeof data === 'object' &&
    data !== null &&
    'error' in data &&
    typeof data.error === 'object' &&
    data.error !== null &&
    'code' in data.error &&
    data.error.code === 'invalid_input'
  );
};

interface ProductDetailsProps {
  attempt: QueueAttempt | null | undefined;
  isAttemptError: boolean;
  isAttemptPending: boolean;
  onJoined: (attempt: QueueAttempt) => void;
  onRetryAttempt: () => void;
  product: Product;
  userId: string | null;
}

function ProductDetails({
  attempt,
  isAttemptError,
  isAttemptPending,
  onJoined,
  onRetryAttempt,
  product,
  userId,
}: ProductDetailsProps) {
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
          <Group gap="xs">
            <ProductAvailabilityBadge product={product} size="sm" variant="light" />
            {product.free_stock > 0 && (
              <Text c="dimmed" size="sm">
                Осталось: {product.free_stock}
              </Text>
            )}
          </Group>
          {product.free_stock === 0 && product.queue_enabled && product.allocatable_stock > 0 && (
            <Text c="dimmed" size="sm">
              Свободных товаров сейчас нет
            </Text>
          )}
          <Text c="dimmed" size="sm">
            {formatProductCategory(product.category)}
          </Text>
          <Text lh={1.55}>{product.description}</Text>
          <Paper mt="xs" p="md" radius="md" withBorder>
            <ProductPurchaseAction
              allocatableStock={product.allocatable_stock}
              attempt={attempt}
              freeStock={product.free_stock}
              isAttemptError={isAttemptError}
              isAttemptPending={isAttemptPending}
              onJoined={onJoined}
              onRetryAttempt={onRetryAttempt}
              productId={product.id}
              queueEnabled={product.queue_enabled}
              userId={userId}
              waitingBufferCapacity={product.waiting_buffer_capacity}
              waitingCount={product.waiting_count}
            />
          </Paper>
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
  const location = useLocation();
  const navigate = useNavigate();
  const { data: product, error, isError, isPending, refetch } = useProductQuery(productId);
  const { userId } = useCurrentDemoUser();
  const queueAttemptQuery = useQueueAttemptQuery(productId, userId);
  const queueNotice = (location.state as { queueNotice?: string } | null)?.queueNotice;

  return (
    <Container size="xl" py={{ base: 'md', sm: 'xl' }}>
      <Stack gap="lg">
        <ProductBreadcrumbs productId={productId} productTitle={product?.title} />

        {queueNotice === 'active-attempt-missing' && (
          <Alert color="blue" title="Эта очередь уже завершилась">
            Вернитесь к товару, чтобы проверить доступность и выбрать следующий шаг.
          </Alert>
        )}

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
          <>
            <ProductDetails
              attempt={queueAttemptQuery.data}
              isAttemptError={userId !== null && queueAttemptQuery.isError}
              isAttemptPending={userId !== null && queueAttemptQuery.isPending}
              onJoined={(attempt) => {
                void navigate(getQueueAttemptRoute(product.id, attempt.state));
              }}
              onRetryAttempt={() => void queueAttemptQuery.refetch()}
              product={product}
              userId={userId}
            />
            <RelevantProducts productId={product.id} />
          </>
        )}
      </Stack>
    </Container>
  );
}
