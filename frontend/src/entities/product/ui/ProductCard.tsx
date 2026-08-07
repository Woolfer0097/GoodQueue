import { AspectRatio, Box, Card, Image, Skeleton, Stack, Text } from '@mantine/core';
import { useState } from 'react';
import { Link } from 'react-router';

import { formatProductPrice, PRODUCT_IMAGE_PLACEHOLDER } from '../model/product.presentation';
import type { Product } from '../model/product.schema';
import { ProductAvailabilityBadge } from './ProductAvailabilityBadge';
import classes from './ProductCard.module.css';

interface ProductCardProps {
  product: Product;
}

export function ProductCard({ product }: ProductCardProps) {
  const imageSource = product.image_url || PRODUCT_IMAGE_PLACEHOLDER;
  const [isImageLoading, setIsImageLoading] = useState(Boolean(product.image_url));

  return (
    <Card
      aria-label={`Открыть товар: ${product.title}`}
      bg="transparent"
      className={classes.card}
      component={Link}
      h="100%"
      padding={0}
      radius={0}
      to={`/products/${product.id}`}
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
          <ProductAvailabilityBadge
            autoContrast
            left="xs"
            pos="absolute"
            product={product}
            size="sm"
            top="xs"
            variant="filled"
          />
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
        <Stack gap={0} mt={2}>
          <Text c="dimmed" lh={1.35} size="xs">
            В наличии: {product.free_stock}
          </Text>
          {product.waiting_count > 0 && (
            <Text c="dimmed" lh={1.35} size="xs">
              В очереди: {product.waiting_count}
            </Text>
          )}
        </Stack>
      </Stack>
    </Card>
  );
}
