import { ActionIcon, Tooltip, useMantineColorScheme } from '@mantine/core';

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
        <span aria-hidden style={{ fontSize: 22, lineHeight: 1 }}>
          {isDark ? '☀' : '☾'}
        </span>
      </ActionIcon>
    </Tooltip>
  );
}
