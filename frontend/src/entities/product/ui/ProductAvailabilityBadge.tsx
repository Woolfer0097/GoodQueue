import { Badge, type BadgeProps } from '@mantine/core';

import { getProductAvailabilityPresentation } from '../model/product.presentation';
import type { Product } from '../model/product.schema';

interface ProductAvailabilityBadgeProps extends Omit<BadgeProps, 'children' | 'color'> {
  product: Product;
}

export function ProductAvailabilityBadge({ product, ...props }: ProductAvailabilityBadgeProps) {
  const availability = getProductAvailabilityPresentation(product);

  return (
    <Badge color={availability.color} {...props}>
      {availability.label}
    </Badge>
  );
}
