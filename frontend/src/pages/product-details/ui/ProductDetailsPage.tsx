import {
  Alert,
  AspectRatio,
  Button,
  Container,
  EmptyState,
  Grid,
  Image,
  SimpleGrid,
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

const isNotFoundError = (error: unknown) =>
  typeof error === 'object' && error !== null && 'status' in error && error.status === 404;

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
          <Text c="dimmed" size="sm">
            {formatProductCategory(product.category)}
          </Text>
          <Text lh={1.55}>{product.description}</Text>
          <SimpleGrid cols={2} mt="xs" spacing="xs" verticalSpacing={4}>
            <Text component="div" size="sm">
              В наличии: {product.free_stock}
            </Text>
            <Text component="div" size="sm">
              В очереди: {product.waiting_count}
            </Text>
            <Text component="div" size="sm">
              Доступно для распределения: {product.allocatable_stock}
            </Text>
            <Text component="div" size="sm">
              Лимит очереди: {product.waiting_buffer_capacity}
            </Text>
          </SimpleGrid>
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
          <Alert color="blue" title="Активная очередь не найдена">
            Возможно, ожидание уже завершилось или вы открыли устаревшую ссылку.
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
