import { MantineProvider } from '@mantine/core';
import { Notifications } from '@mantine/notifications';
import { QueryClientProvider } from '@tanstack/react-query';
import { ReactQueryDevtools } from '@tanstack/react-query-devtools';
import type { PropsWithChildren } from 'react';
import { BrowserRouter } from 'react-router';

import { CurrentDemoUserProvider } from '@/entities/demo-user';
import { QueueAttemptNotifications } from '@/features/queue-attempt-notifications';

import { theme } from '../theme/theme';
import { queryClient } from './query-client';
import { QueryNavigationProgress } from './QueryNavigationProgress';

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <QueryClientProvider client={queryClient}>
      <CurrentDemoUserProvider>
        <MantineProvider theme={theme}>
          <QueryNavigationProgress />
          <BrowserRouter>
            <Notifications limit={Number.POSITIVE_INFINITY} position="top-right" />
            <QueueAttemptNotifications />
            {children}
          </BrowserRouter>
          {import.meta.env?.DEV && <ReactQueryDevtools initialIsOpen={false} />}
        </MantineProvider>
      </CurrentDemoUserProvider>
    </QueryClientProvider>
  );
}
