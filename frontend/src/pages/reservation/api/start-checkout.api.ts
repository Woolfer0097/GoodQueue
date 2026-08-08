import { queueAttemptSchema } from '@/entities/queue-attempt';
import { apiClient } from '@/shared/api';

export const startCheckout = async (attemptId: string, userId: string) => {
  const response = await apiClient(
    `/api/v1/queue-attempts/${encodeURIComponent(attemptId)}/checkout`,
    {
      headers: { 'X-User-ID': userId },
      method: 'POST',
    },
  );

  return queueAttemptSchema.parse(response);
};
