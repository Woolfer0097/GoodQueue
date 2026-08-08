import {
  Alert,
  Avatar,
  Box,
  Button,
  EmptyState,
  Group,
  Loader,
  Popover,
  Select,
  Stack,
  Text,
} from '@mantine/core';
import { IconChevronUp } from '@tabler/icons-react';

import { useCurrentDemoUser, useDemoUsersQuery } from '@/entities/demo-user';

export function DemoUserSelect() {
  const { currentUser, selectUser, userId } = useCurrentDemoUser();
  const { data: users, isError, isPending } = useDemoUsersQuery();
  const isEmpty = !isPending && !isError && users?.length === 0;

  const triggerLabel = isPending
    ? 'Загрузка аккаунта'
    : isError
      ? 'Аккаунты недоступны'
      : isEmpty
        ? 'Нет аккаунтов'
        : (currentUser?.display_name ?? 'Аккаунт');
  const accessibleStatus = isPending
    ? 'загрузка'
    : isError
      ? 'ошибка загрузки'
      : isEmpty
        ? 'список пуст'
        : triggerLabel;
  const avatarColor = isError ? 'red' : isEmpty ? 'gray' : 'avitoBlue';
  const options =
    users?.map((user) => ({
      label: user.display_name,
      value: user.external_user_id,
    })) ?? [];

  return (
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
          aria-label={`Сменить аккаунт: ${accessibleStatus}`}
          h="auto"
          p={6}
          pr="sm"
          radius="xl"
          variant="default"
        >
          <Group gap="xs" wrap="nowrap">
            <Avatar color={avatarColor} name={currentUser?.display_name} radius="xl">
              {isPending ? <Loader size={18} /> : undefined}
            </Avatar>
            <Box ta="left">
              <Text fw={700} lh={1.2} size="sm">
                {triggerLabel}
              </Text>
              <Text c="dimmed" lh={1.2} size="xs">
                Аккаунт
              </Text>
            </Box>
            <IconChevronUp aria-hidden color="var(--mantine-color-dimmed)" size={16} />
          </Group>
        </Button>
      </Popover.Target>

      <Popover.Dropdown>
        <Stack gap="sm">
          <Text fw={700} size="sm">
            Выберите активный аккаунт
          </Text>

          {isError ? (
            <Alert color="red" title="Не удалось загрузить аккаунты" variant="light">
              Обновите страницу и попробуйте ещё раз.
            </Alert>
          ) : isEmpty ? (
            <EmptyState
              description="Сейчас нет доступных аккаунтов."
              size="xs"
              title="Аккаунты не найдены"
            />
          ) : (
            <Select<string>
              allowDeselect={false}
              aria-label="Аккаунт"
              comboboxProps={{ withinPortal: false }}
              data={options}
              disabled={isPending}
              loading={isPending}
              nothingFoundMessage="Аккаунты не найдены"
              onChange={(value) => {
                if (value !== null) {
                  selectUser(value);
                }
              }}
              placeholder={isPending ? 'Загрузка аккаунтов…' : 'Выберите аккаунт'}
              value={userId}
            />
          )}
        </Stack>
      </Popover.Dropdown>
    </Popover>
  );
}
