import { useQuery } from '@tanstack/react-query';

import { getDemoUsers } from './demo-user.api';
import { demoUserQueryKeys } from './demo-user.query-keys';

export const useDemoUsersQuery = () =>
  useQuery({
    queryFn: getDemoUsers,
    queryKey: demoUserQueryKeys.list(),
  });
