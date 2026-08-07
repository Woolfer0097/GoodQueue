import { queueAttemptSchema } from '@/entities/queue-attempt';
import { apiClient } from '@/shared/api';

export const cancelQueueAttempt = async (productId: string, userId: string) => {
  const response = await apiClient(
    `/api/v1/products/${encodeURIComponent(productId)}/queue-entry`,
    {
      headers: { 'X-User-ID': userId },
      method: 'DELETE',
    },
  );

  if (response === undefined) {
    return undefined;
  }

  return queueAttemptSchema.parse(response);
};
