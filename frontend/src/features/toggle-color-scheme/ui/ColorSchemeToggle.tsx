import { ActionIcon, Tooltip, useMantineColorScheme } from '@mantine/core';
import { IconMoon, IconSun } from '@tabler/icons-react';

export function ColorSchemeToggle() {
  const { colorScheme, toggleColorScheme } = useMantineColorScheme();
  const isDark = colorScheme === 'dark';
  const label = isDark ? 'Включить светлую тему' : 'Включить тёмную тему';

  return (
    <Tooltip label={label} position="top" withArrow>
      <ActionIcon
        aria-label={label}
        onClick={toggleColorScheme}
        radius="xl"
        size={50}
        variant="default"
      >
        {isDark ? <IconSun aria-hidden size={22} /> : <IconMoon aria-hidden size={22} />}
      </ActionIcon>
    </Tooltip>
  );
}
