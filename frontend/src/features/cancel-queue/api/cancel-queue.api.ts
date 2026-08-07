import { apiClient } from '@/shared/api';

export const cancelQueueAttempt = async (productId: string, userId: string) => {
  await apiClient(`/api/v1/products/${encodeURIComponent(productId)}/queue-entry`, {
    headers: { 'X-User-ID': userId },
    method: 'DELETE',
  });
};
