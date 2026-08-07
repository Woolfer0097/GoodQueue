import {
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
import { Link, useLocation, useNavigate, useParams } from 'react-router';

import {
  formatProductPrice,
  type Product,
  PRODUCT_IMAGE_PLACEHOLDER,
  ProductAvailabilityBadge,
  useProductQuery,
} from '@/entities/product';
import { getQueueAttemptRoute, type QueueAttempt } from '@/entities/queue-attempt';
import { JoinQueueButton } from '@/features/join-queue';
import { useCurrentDemoUser } from '@/features/select-demo-user';
import { ProductBreadcrumbs } from '@/widgets/product-breadcrumbs';

const isNotFoundError = (error: unknown) =>
  typeof error === 'object' && error !== null && 'status' in error && error.status === 404;

interface ProductDetailsProps {
  onJoined: (attempt: QueueAttempt) => void;
  product: Product;
  userId: string | null;
}

function ProductDetails({ onJoined, product, userId }: ProductDetailsProps) {
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
          <JoinQueueButton onJoined={onJoined} productId={product.id} userId={userId} />
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
          <ProductDetails
            onJoined={(attempt) => {
              void navigate(getQueueAttemptRoute(product.id, attempt.state));
            }}
            product={product}
            userId={userId}
          />
        )}
      </Stack>
    </Container>
  );
}
