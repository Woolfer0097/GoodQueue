import { AspectRatio, Badge, Box, Card, Image, Skeleton, Stack, Text } from '@mantine/core';
import { useState } from 'react';
import { Link } from 'react-router';

import { getProductAvailability, type ProductAvailability } from '../model/product.availability';
import type { Product } from '../model/product.schema';

interface ProductCardProps {
  product: Product;
}

const PRODUCT_PLACEHOLDER =
  'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 viewBox=%220 0 800 600%22%3E%3Crect width=%22800%22 height=%22600%22 fill=%22%23f1f3f5%22/%3E%3Cpath d=%22M300 390l75-90 55 65 45-50 75 75H300z%22 fill=%22%23ced4da%22/%3E%3Ccircle cx=%22345%22 cy=%22245%22 r=%2232%22 fill=%22%23ced4da%22/%3E%3C/svg%3E';

const availabilityPresentation: Record<ProductAvailability, { color: string; label: string }> = {
  available: { color: 'green', label: 'В наличии' },
  available_by_queue: { color: 'yellow', label: 'Доступно по очереди' },
  sold_out: { color: 'gray', label: 'Нет в наличии' },
  unavailable: { color: 'gray', label: 'Покупка временно недоступна' },
};

const priceFormatter = new Intl.NumberFormat('ru-RU', {
  currency: 'RUB',
  maximumFractionDigits: 2,
  minimumFractionDigits: 0,
  style: 'currency',
});

const formatPrice = (priceCents: number) =>
  priceFormatter.format(priceCents / 100).replaceAll('\u00a0', ' ');

export function ProductCard({ product }: ProductCardProps) {
  const availability = availabilityPresentation[getProductAvailability(product)];
  const imageSource = product.image_url || PRODUCT_PLACEHOLDER;
  const [isFocused, setIsFocused] = useState(false);
  const [isHovered, setIsHovered] = useState(false);
  const [isImageLoading, setIsImageLoading] = useState(Boolean(product.image_url));
  const isHighlighted = isFocused || isHovered;

  return (
    <Card
      aria-label={`Открыть товар: ${product.title}`}
      bg="transparent"
      component={Link}
      h="100%"
      onBlur={() => setIsFocused(false)}
      onFocus={() => setIsFocused(true)}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      padding={0}
      radius={0}
      style={{
        color: 'inherit',
        outline: isFocused ? '2px solid var(--mantine-color-avitoBlue-6)' : 'none',
        outlineOffset: 4,
        textDecoration: 'none',
      }}
      to={`/products/${product.id}`}
    >
      <AspectRatio ratio={1}>
        <Box
          pos="relative"
          style={{ borderRadius: 'var(--mantine-radius-md)', overflow: 'hidden' }}
        >
          <Skeleton
            data-testid="product-image-skeleton"
            h="100%"
            radius="md"
            visible={isImageLoading}
          >
            <Image
              alt={product.title}
              fallbackSrc={PRODUCT_PLACEHOLDER}
              fit="cover"
              h="100%"
              onError={() => setIsImageLoading(false)}
              onLoad={() => setIsImageLoading(false)}
              src={imageSource}
              style={{
                transform: isHighlighted ? 'scale(1.02)' : 'scale(1)',
                transition: 'transform 150ms ease',
              }}
              w="100%"
            />
          </Skeleton>
          <Badge
            autoContrast
            color={availability.color}
            left="xs"
            pos="absolute"
            size="sm"
            top="xs"
            variant="filled"
          >
            {availability.label}
          </Badge>
        </Box>
      </AspectRatio>

      <Stack gap={4} mt="xs">
        <Text
          component="h2"
          fw={400}
          lineClamp={2}
          lh={1.3}
          m={0}
          size="sm"
          style={{
            color: isHighlighted ? 'var(--mantine-color-avitoBlue-7)' : 'inherit',
            transition: 'color 150ms ease',
          }}
        >
          {product.title}
        </Text>
        <Text fw={700} lh={1.2} size="lg">
          {formatPrice(product.price_cents)}
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
