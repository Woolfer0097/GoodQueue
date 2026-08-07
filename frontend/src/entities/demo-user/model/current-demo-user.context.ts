import { createContext } from 'react';

import type { DemoUser } from './demo-user.schema';

export interface CurrentDemoUserContextValue {
  currentUser: DemoUser | null;
  selectUser: (userId: string) => void;
  userId: string | null;
}

export const CurrentDemoUserContext = createContext<CurrentDemoUserContextValue | null>(null);
