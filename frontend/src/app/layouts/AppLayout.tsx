import { AppShell } from '@mantine/core';
import { Outlet } from 'react-router';

import { DemoUserSelect } from '@/features/select-demo-user';

export function AppLayout() {
  return (
    <AppShell padding="md">
      <AppShell.Main>
        <Outlet />
      </AppShell.Main>
      <DemoUserSelect />
    </AppShell>
  );
}
