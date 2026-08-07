import { queueAttemptSchema } from '@/entities/queue-attempt';
import { apiClient } from '@/shared/api';

export const joinQueue = async (productId: string, userId: string, idempotencyKey: string) => {
  const response = await apiClient(
    `/api/v1/products/${encodeURIComponent(productId)}/queue-entries`,
    {
      headers: {
        'Idempotency-Key': idempotencyKey,
        'X-User-ID': userId,
      },
      method: 'POST',
    },
  );

  return queueAttemptSchema.parse(response);
};
