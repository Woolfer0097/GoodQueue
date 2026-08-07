import type { Product } from './product.schema';

export type ProductAvailability = 'available' | 'available_by_queue' | 'sold_out' | 'unavailable';

type AvailabilitySource = Pick<Product, 'allocatable_stock' | 'free_stock' | 'queue_enabled'>;

export const getProductAvailability = ({
  allocatable_stock,
  free_stock,
  queue_enabled,
}: AvailabilitySource): ProductAvailability => {
  if (!queue_enabled) {
    return 'unavailable';
  }

  if (free_stock > 0) {
    return 'available';
  }

  if (allocatable_stock > 0) {
    return 'available_by_queue';
  }

  return 'sold_out';
};
