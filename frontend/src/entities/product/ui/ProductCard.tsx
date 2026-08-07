import { AspectRatio, Badge, Button, Card, Group, Image, Stack, Text } from '@mantine/core';
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

  return (
    <Card padding="lg" radius="md" withBorder h="100%">
      <Card.Section>
        <AspectRatio ratio={4 / 3}>
          <Image
            alt={product.title}
            fallbackSrc={PRODUCT_PLACEHOLDER}
            fit="cover"
            src={imageSource}
          />
        </AspectRatio>
      </Card.Section>

      <Stack gap="sm" mt="md" h="100%">
        <Text component="h3" fw={600} lineClamp={2} m={0} size="lg">
          {product.title}
        </Text>
        <Text fw={700} size="xl">
          {formatPrice(product.price_cents)}
        </Text>
        <Badge color={availability.color} variant="light" w="fit-content">
          {availability.label}
        </Badge>
        <Group gap="xs" justify="space-between">
          <Text c="dimmed" size="sm">
            Свободный остаток: {product.free_stock}
          </Text>
          <Text c="dimmed" size="sm">
            В очереди: {product.waiting_count}
          </Text>
        </Group>
        <Button component={Link} fullWidth mt="auto" to={`/products/${product.id}`} variant="light">
          Открыть товар
        </Button>
      </Stack>
    </Card>
  );
}
