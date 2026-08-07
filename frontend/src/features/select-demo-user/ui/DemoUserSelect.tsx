import {
  Affix,
  Alert,
  Avatar,
  Badge,
  Box,
  Button,
  EmptyState,
  Group,
  Indicator,
  Loader,
  Popover,
  Select,
  Stack,
  Text,
} from '@mantine/core';

import { useCurrentDemoUser, useDemoUsersQuery } from '@/entities/demo-user';

export function DemoUserSelect() {
  const { currentUser, selectUser, userId } = useCurrentDemoUser();
  const { data: users, isError, isPending } = useDemoUsersQuery();
  const isEmpty = !isPending && !isError && users?.length === 0;

  const triggerLabel = isPending
    ? 'Загрузка пользователя'
    : isError
      ? 'Профиль недоступен'
      : isEmpty
        ? 'Нет demo-профилей'
        : (currentUser?.display_name ?? 'Demo-пользователь');
  const accessibleStatus = isPending
    ? 'загрузка'
    : isError
      ? 'ошибка загрузки'
      : isEmpty
        ? 'список пуст'
        : triggerLabel;
  const indicatorColor = isError ? 'red' : isEmpty ? 'gray' : 'avitoBlue';
  const options =
    users?.map((user) => ({
      label: user.display_name,
      value: user.external_user_id,
    })) ?? [];

  return (
    <Affix position={{ bottom: 20, right: 20 }} zIndex={200}>
      <Popover
        keepMounted
        position="top-end"
        shadow="lg"
        trapFocus
        width={320}
        withinPortal={false}
        withArrow
      >
        <Popover.Target>
          <Button
            aria-label={`Настроить demo-пользователя: ${accessibleStatus}`}
            h="auto"
            p={6}
            pr="sm"
            radius="xl"
            variant="default"
          >
            <Group gap="xs" wrap="nowrap">
              <Indicator
                color={indicatorColor}
                disabled={isEmpty}
                offset={4}
                processing={isPending}
                size={10}
                withBorder
              >
                <Avatar color={indicatorColor} name={currentUser?.display_name} radius="xl">
                  {isPending ? <Loader size={18} /> : undefined}
                </Avatar>
              </Indicator>
              <Box ta="left">
                <Text fw={700} lh={1.2} size="sm">
                  {triggerLabel}
                </Text>
                <Text c="dimmed" lh={1.2} size="xs">
                  Demo-профиль
                </Text>
              </Box>
              <Text aria-hidden c="dimmed" size="sm">
                ⌃
              </Text>
            </Group>
          </Button>
        </Popover.Target>

        <Popover.Dropdown>
          <Stack gap="sm">
            <Box>
              <Group gap="xs" justify="space-between">
                <Text fw={700} size="sm">
                  Demo-пользователь
                </Text>
                <Badge color="avitoBlue" variant="light">
                  Demo
                </Badge>
              </Group>
              <Text c="dimmed" mt={4} size="xs">
                Выберите тестовый профиль для демонстрации пользовательских сценариев.
              </Text>
            </Box>

            {isError ? (
              <Alert color="red" title="Не удалось загрузить demo-пользователей" variant="light">
                Обновите страницу или проверьте доступность backend.
              </Alert>
            ) : isEmpty ? (
              <EmptyState
                description="Backend не вернул доступных пользователей."
                size="xs"
                title="Demo-пользователи не найдены"
              />
            ) : (
              <Select<string>
                allowDeselect={false}
                comboboxProps={{ withinPortal: false }}
                data={options}
                disabled={isPending}
                label="Текущий demo-пользователь"
                loading={isPending}
                nothingFoundMessage="Demo-пользователи не найдены"
                onChange={(value) => {
                  if (value !== null) {
                    selectUser(value);
                  }
                }}
                placeholder={isPending ? 'Загрузка пользователей…' : 'Выберите пользователя'}
                value={userId}
              />
            )}
          </Stack>
        </Popover.Dropdown>
      </Popover>
    </Affix>
  );
}
