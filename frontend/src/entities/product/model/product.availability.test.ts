import { getProductAvailability } from './product.availability';

describe('getProductAvailability', () => {
  it.each([
    [
      {
        allocatable_stock: 3,
        free_stock: 0,
        queue_enabled: true,
        waiting_buffer_capacity: 3,
        waiting_count: 2,
      },
      'available_by_queue',
    ],
    [
      {
        allocatable_stock: 3,
        free_stock: 0,
        queue_enabled: true,
        waiting_buffer_capacity: 3,
        waiting_count: 3,
      },
      'queue_full',
    ],
  ] as const)('returns %s as %s', (product, availability) => {
    expect(getProductAvailability(product)).toBe(availability);
  });
});
