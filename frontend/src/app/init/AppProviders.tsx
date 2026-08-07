import { MantineProvider } from '@mantine/core';
import { Notifications } from '@mantine/notifications';
import { QueryClientProvider } from '@tanstack/react-query';
import { ReactQueryDevtools } from '@tanstack/react-query-devtools';
import type { PropsWithChildren } from 'react';
import { BrowserRouter } from 'react-router';

import { theme } from '../theme/theme';
import { queryClient } from './query-client';
import { QueryNavigationProgress } from './QueryNavigationProgress';

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <QueryClientProvider client={queryClient}>
      <MantineProvider theme={theme}>
        <QueryNavigationProgress />
        <Notifications limit={3} position="top-right" />
        <BrowserRouter>{children}</BrowserRouter>
        {import.meta.env?.DEV && <ReactQueryDevtools initialIsOpen={false} />}
      </MantineProvider>
    </QueryClientProvider>
  );
}
