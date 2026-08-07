import { apiClient, ApiError } from '@/shared/api';

import { queueAttemptSchema } from '../model/queue-attempt.schema';

export const getQueueAttempt = async (productId: string, userId: string) => {
  try {
    const response = await apiClient(
      `/api/v1/products/${encodeURIComponent(productId)}/queue-entry`,
      { headers: { 'X-User-ID': userId } },
    );

    return queueAttemptSchema.parse(response);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) {
      return null;
    }

    throw error;
  }
};
