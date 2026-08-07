import { z } from 'zod';

import { nonEmptyStringSchema } from '@/shared/lib/validation';

const nonNegativeIntegerSchema = z.number().int().nonnegative();

export const productSchema = z.object({
  allocatable_stock: nonNegativeIntegerSchema,
  category: z.string(),
  description: z.string(),
  free_stock: nonNegativeIntegerSchema,
  // TODO: Use uuidSchema when backend product IDs have RFC-compliant version and variant bits.
  id: z.guid(),
  image_url: z.url(),
  price_cents: nonNegativeIntegerSchema,
  queue_enabled: z.boolean(),
  reserved: nonNegativeIntegerSchema,
  title: nonEmptyStringSchema,
  waiting_buffer_capacity: nonNegativeIntegerSchema,
  waiting_count: nonNegativeIntegerSchema,
});

export const productListSchema = z.array(productSchema);

export type Product = z.infer<typeof productSchema>;
