import { z } from 'zod';

import { nonEmptyStringSchema } from '@/shared/lib/validation';

const nonNegativeIntegerSchema = z.number().int().nonnegative();
const positiveIntegerSchema = z.number().int().positive();
const dateTimeSchema = z.iso.datetime({ offset: true });

export const queueAttemptStateSchema = z.enum([
  'waiting',
  'invited',
  'checkout',
  'purchased',
  'invite_expired',
  'checkout_expired',
  'payment_failed',
  'cancelled',
  'sold_out',
]);

export const queueAttemptSchema = z.object({
  attempt_id: z.guid(),
  checkout_started_at: dateTimeSchema.optional(),
  created_at: dateTimeSchema,
  deadline_at: dateTimeSchema.optional(),
  expires_at: dateTimeSchema.optional(),
  invited_at: dateTimeSchema.optional(),
  message_code: nonEmptyStringSchema,
  next_action: nonEmptyStringSchema,
  position: positiveIntegerSchema.optional(),
  position_ahead: nonNegativeIntegerSchema.optional(),
  product_id: z.guid(),
  purchased_at: dateTimeSchema.optional(),
  queue_sequence: nonNegativeIntegerSchema,
  state: queueAttemptStateSchema,
  terminal_at: dateTimeSchema.optional(),
  total_waiting: nonNegativeIntegerSchema.optional(),
  updated_at: dateTimeSchema,
});

export type QueueAttempt = z.infer<typeof queueAttemptSchema>;
export type QueueAttemptState = z.infer<typeof queueAttemptStateSchema>;
