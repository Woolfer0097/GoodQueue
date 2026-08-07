import { z } from 'zod';

import { nonEmptyStringSchema, uuidSchema } from '@/shared/lib/validation';

export const demoUserSchema = z.object({
  display_name: nonEmptyStringSchema,
  external_user_id: uuidSchema,
});

export const demoUserListSchema = z.array(demoUserSchema);

export type DemoUser = z.infer<typeof demoUserSchema>;
