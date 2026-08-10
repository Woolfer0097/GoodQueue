import { z } from 'zod';

import { nonEmptyStringSchema } from '@/shared/lib/validation';

const nonNegativeIntegerSchema = z.number().int().nonnegative();
const productImageUrlSchema = z
  .string()
  .refine(
    (value) =>
      value === '' || value.startsWith('/product-images/') || z.url().safeParse(value).success,
    'Expected an empty value, an absolute URL or a local product image path',
  );

export const productSchema = z.object({
  allocatable_stock: nonNegativeIntegerSchema,
  category: z.string(),
  description: z.string(),
  free_stock: nonNegativeIntegerSchema,
  // TODO: Use uuidSchema when backend product IDs have RFC-compliant version and variant bits.
  id: z.guid(),
  image_url: productImageUrlSchema,
  price_cents: nonNegativeIntegerSchema,
  queue_enabled: z.boolean(),
  reserved: nonNegativeIntegerSchema,
  title: nonEmptyStringSchema,
  waiting_buffer_capacity: nonNegativeIntegerSchema,
  waiting_count: nonNegativeIntegerSchema,
});

export const productListSchema = z.array(productSchema);

export const productAlternativesSchema = z.array(
  productSchema.extend({
    reason_code: z
      .enum(['semantically_similar', 'same_category_available', 'available_now'])
      .optional(),
    recommendation_mode: z.enum(['ai_semantic', 'catalog_fallback']).optional(),
    recommendation_score: z.number().min(0).max(1).optional(),
  }),
);

export type Product = z.infer<typeof productSchema>;
export type ProductAlternative = z.infer<typeof productAlternativesSchema>[number];
