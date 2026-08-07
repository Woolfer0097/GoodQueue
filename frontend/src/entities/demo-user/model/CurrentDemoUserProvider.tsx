import { type PropsWithChildren, useCallback, useEffect, useState } from 'react';

import { useDemoUsersQuery } from '../api/demo-user.queries';
import { CurrentDemoUserContext } from './current-demo-user.context';
import { readStoredDemoUserId, storeDemoUserId } from './demo-user.storage';

export function CurrentDemoUserProvider({ children }: PropsWithChildren) {
  const { data: users } = useDemoUsersQuery();
  const [preferredUserId, setPreferredUserId] = useState(readStoredDemoUserId);
  const currentUser =
    users?.find((user) => user.external_user_id === preferredUserId) ?? users?.[0] ?? null;

  useEffect(() => {
    if (users === undefined) {
      return;
    }

    storeDemoUserId(currentUser?.external_user_id ?? null);
  }, [currentUser, users]);

  const selectUser = useCallback(
    (userId: string) => {
      if (users?.some((user) => user.external_user_id === userId)) {
        setPreferredUserId(userId);
      }
    },
    [users],
  );

  return (
    <CurrentDemoUserContext.Provider
      value={{ currentUser, selectUser, userId: currentUser?.external_user_id ?? null }}
    >
      {children}
    </CurrentDemoUserContext.Provider>
  );
}
