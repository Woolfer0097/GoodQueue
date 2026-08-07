import { createApiClient } from './client';
import { API_BASE_URL } from './config';

export { ApiError } from './api-error';

export const apiClient = createApiClient(API_BASE_URL);
