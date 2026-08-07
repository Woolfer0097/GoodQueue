import { NavigationProgress, nprogress } from '@mantine/nprogress';
import { useIsFetching, useIsMutating } from '@tanstack/react-query';
import { useEffect, useRef } from 'react';

export function QueryNavigationProgress() {
  const activeQueryCount = useIsFetching({
    predicate: (query) => query.meta?.background !== true,
  });
  const activeMutationCount = useIsMutating();
  const wasActive = useRef(false);
  const isActive = activeQueryCount + activeMutationCount > 0;

  useEffect(() => {
    if (isActive) {
      nprogress.start();
    } else if (wasActive.current) {
      nprogress.complete();
    }

    wasActive.current = isActive;
  }, [isActive]);

  return <NavigationProgress />;
}
