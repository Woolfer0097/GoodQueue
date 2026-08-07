import { apiClient } from '@/shared/api';

import { demoUserListSchema } from '../model/demo-user.schema';

export const getDemoUsers = async () => {
  const response = await apiClient('/api/v1/demo/users');

  return demoUserListSchema.parse(response);
};
