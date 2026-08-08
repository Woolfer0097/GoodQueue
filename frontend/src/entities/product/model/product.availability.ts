import type { Product } from './product.schema';

export type ProductAvailability =
  'available' | 'available_by_queue' | 'queue_full' | 'sold_out' | 'unavailable';

type AvailabilitySource = Pick<
  Product,
  'allocatable_stock' | 'free_stock' | 'queue_enabled' | 'waiting_buffer_capacity' | 'waiting_count'
>;

export const getProductAvailability = ({
  allocatable_stock,
  free_stock,
  queue_enabled,
  waiting_buffer_capacity,
  waiting_count,
}: AvailabilitySource): ProductAvailability => {
  if (!queue_enabled) {
    return 'unavailable';
  }

  if (allocatable_stock === 0) {
    return 'sold_out';
  }

  if (free_stock > 0) {
    return 'available';
  }

  return waiting_count >= waiting_buffer_capacity ? 'queue_full' : 'available_by_queue';
};
