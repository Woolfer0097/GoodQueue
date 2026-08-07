import { AppShell } from '@mantine/core';
import { Outlet } from 'react-router';

export function AppLayout() {
  return (
    <AppShell padding="md">
      <AppShell.Main>
        <Outlet />
      </AppShell.Main>
    </AppShell>
  );
}
