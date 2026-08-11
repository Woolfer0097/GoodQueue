import { AspectRatio, Badge, Box, Card, Group, Image, Skeleton, Stack, Text } from '@mantine/core';
import { useState } from 'react';
import { Link } from 'react-router';

import { formatProductPrice, PRODUCT_IMAGE_PLACEHOLDER } from '../model/product.presentation';
import type { Product } from '../model/product.schema';
import { ProductAvailabilityBadge } from './ProductAvailabilityBadge';
import classes from './ProductCard.module.css';

interface ProductCardProps {
  product: Product;
  userStatus?: ProductCardUserStatus;
}

export interface ProductCardUserStatus {
  href: string;
  label: string;
  tone: 'checkout' | 'ready' | 'waiting';
}

const statusColors: Record<ProductCardUserStatus['tone'], string> = {
  checkout: 'violet',
  ready: 'green',
  waiting: 'avitoBlue',
};

const getQueueLabel = (waitingCount: number) =>
  waitingCount > 0 ? `В очереди: ${waitingCount}` : 'Очереди нет';

export function ProductCard({ product, userStatus }: ProductCardProps) {
  const imageSource = product.image_url || PRODUCT_IMAGE_PLACEHOLDER;
  const [isImageLoading, setIsImageLoading] = useState(Boolean(product.image_url));

  return (
    <Card
      aria-label={
        userStatus
          ? `Продолжить покупку: ${product.title}. ${userStatus.label}`
          : `Открыть товар: ${product.title}`
      }
      bg="transparent"
      className={`${classes.card}${userStatus ? ` ${classes.activeQueue}` : ''}`}
      component={Link}
      data-active-queue={userStatus ? 'true' : undefined}
      h="100%"
      padding={0}
      radius={0}
      to={userStatus?.href ?? `/products/${product.id}`}
    >
      <AspectRatio ratio={1}>
        <Box className={classes.imageFrame} pos="relative">
          <Skeleton
            data-testid="product-image-skeleton"
            h="100%"
            radius="md"
            visible={isImageLoading}
          >
            <Image
              alt={product.title}
              fallbackSrc={PRODUCT_IMAGE_PLACEHOLDER}
              fit="cover"
              h="100%"
              className={classes.image}
              onError={() => setIsImageLoading(false)}
              onLoad={() => setIsImageLoading(false)}
              src={imageSource}
              w="100%"
            />
          </Skeleton>
        </Box>
      </AspectRatio>

      <Stack gap={4} mt="xs">
        <Text
          component="h2"
          className={classes.title}
          fw={400}
          lineClamp={2}
          lh={1.3}
          m={0}
          size="sm"
        >
          {product.title}
        </Text>
        <Text fw={700} lh={1.2} size="lg">
          {formatProductPrice(product.price_cents)}
        </Text>
        <Group gap={6} justify="space-between" wrap="wrap">
          <ProductAvailabilityBadge product={product} size="xs" variant="light" w="fit-content" />
          <Badge
            color={product.waiting_count > 0 ? 'avitoBlue' : 'gray'}
            data-testid="product-queue-count"
            size="xs"
            variant="light"
            w="fit-content"
          >
            {getQueueLabel(product.waiting_count)}
          </Badge>
        </Group>
        {userStatus ? (
          <Badge
            color={statusColors[userStatus.tone]}
            data-testid="product-queue-status"
            size="sm"
            variant="filled"
            w="fit-content"
          >
            {userStatus.label}
          </Badge>
        ) : null}
      </Stack>
    </Card>
  );
}
