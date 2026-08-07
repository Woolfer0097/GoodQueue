import { useContext } from 'react';

import { CurrentDemoUserContext } from './current-demo-user.context';

export const useCurrentDemoUser = () => {
  const currentDemoUser = useContext(CurrentDemoUserContext);

  if (currentDemoUser === null) {
    throw new Error('useCurrentDemoUser must be used within CurrentDemoUserProvider');
  }

  return currentDemoUser;
};
