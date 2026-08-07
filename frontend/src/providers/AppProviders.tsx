import { MantineProvider } from '@mantine/core';
import { Notifications } from '@mantine/notifications';
import type { PropsWithChildren } from 'react';
import { BrowserRouter } from 'react-router';

import { theme } from '@/theme';

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <MantineProvider theme={theme}>
      <Notifications limit={3} position="top-right" />
      <BrowserRouter>{children}</BrowserRouter>
    </MantineProvider>
  );
}
