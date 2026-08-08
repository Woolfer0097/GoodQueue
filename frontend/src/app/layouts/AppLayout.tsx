import { Affix, AppShell, Group } from '@mantine/core';
import { Outlet } from 'react-router';

import { DemoUserSelect } from '@/features/select-demo-user';
import { ColorSchemeToggle } from '@/features/toggle-color-scheme';

import classes from './AppLayout.module.css';

export function AppLayout() {
  return (
    <AppShell padding="md">
      <AppShell.Main className={classes.main}>
        <Outlet />
      </AppShell.Main>
      <Affix position={{ bottom: 20, right: 20 }} zIndex={200}>
        <Group align="center" gap="xs" wrap="nowrap">
          <ColorSchemeToggle />
          <DemoUserSelect />
        </Group>
      </Affix>
    </AppShell>
  );
}
