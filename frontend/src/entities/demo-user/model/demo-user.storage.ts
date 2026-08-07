import { uuidSchema } from '@/shared/lib/validation';

const DEMO_USER_STORAGE_KEY = 'goodqueue.demo-user-id';

export const readStoredDemoUserId = () => {
  try {
    const storedUserId = localStorage.getItem(DEMO_USER_STORAGE_KEY);
    const parsedUserId = uuidSchema.safeParse(storedUserId);

    if (parsedUserId.success) {
      return parsedUserId.data;
    }

    localStorage.removeItem(DEMO_USER_STORAGE_KEY);
    return null;
  } catch {
    return null;
  }
};

export const storeDemoUserId = (userId: string | null) => {
  try {
    if (userId === null) {
      localStorage.removeItem(DEMO_USER_STORAGE_KEY);
      return;
    }

    localStorage.setItem(DEMO_USER_STORAGE_KEY, userId);
  } catch {
    // Selection remains available in memory when browser storage is unavailable.
  }
};
