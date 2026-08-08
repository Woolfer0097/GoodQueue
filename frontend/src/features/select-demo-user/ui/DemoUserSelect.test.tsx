import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const useCurrentDemoUserMock = jest.fn();
const useDemoUsersQueryMock = jest.fn();
const selectUserMock = jest.fn();

jest.unstable_mockModule('@/entities/demo-user', () => ({
  useCurrentDemoUser: useCurrentDemoUserMock,
  useDemoUsersQuery: useDemoUsersQueryMock,
}));

const { DemoUserSelect } = await import('./DemoUserSelect');

const users = [
  {
    display_name: 'Пользователь 1',
    external_user_id: '00000000-0000-4000-8000-000000000001',
  },
  {
    display_name: 'Пользователь 2',
    external_user_id: '00000000-0000-4000-8000-000000000002',
  },
];

const renderSelect = () =>
  render(
    <MantineProvider>
      <DemoUserSelect />
    </MantineProvider>,
  );

describe('DemoUserSelect', () => {
  beforeEach(() => {
    selectUserMock.mockReset();
    useCurrentDemoUserMock.mockReset();
    useDemoUsersQueryMock.mockReset();
    useCurrentDemoUserMock.mockReturnValue({
      currentUser: users[0],
      selectUser: selectUserMock,
      userId: users[0].external_user_id,
    });
    useDemoUsersQueryMock.mockReturnValue({ data: users, isError: false, isPending: false });
  });

  it('loads options and presents the current user as an account', async () => {
    const user = userEvent.setup();
    renderSelect();

    const trigger = screen.getByRole('button', {
      name: `Сменить аккаунт: ${users[0].display_name}`,
    });
    expect(trigger).toHaveTextContent(users[0].display_name);
    expect(trigger).toHaveTextContent('Аккаунт');

    await user.click(trigger);
    expect(trigger).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText('Выберите аккаунт')).toBeInTheDocument();
    expect(screen.queryByText(/demo|backend/i)).not.toBeInTheDocument();

    const select = screen.getByRole('combobox', { hidden: true });
    expect(select).toHaveValue(users[0].display_name);

    fireEvent.click(select);

    await waitFor(() => {
      expect(screen.getAllByRole('option', { hidden: true })).toHaveLength(users.length);
    });
    expect(screen.getAllByRole('option', { hidden: true })[0]).toHaveTextContent(
      users[0].display_name,
    );
    expect(screen.getAllByRole('option', { hidden: true })[1]).toHaveTextContent(
      users[1].display_name,
    );
  });

  it('switches the current account', async () => {
    const user = userEvent.setup();
    renderSelect();

    await user.click(
      screen.getByRole('button', {
        name: `Сменить аккаунт: ${users[0].display_name}`,
      }),
    );
    fireEvent.click(screen.getByRole('combobox', { hidden: true }));
    await waitFor(() => {
      expect(screen.getAllByRole('option', { hidden: true })).toHaveLength(users.length);
    });
    fireEvent.click(screen.getAllByRole('option', { hidden: true })[1]);

    expect(selectUserMock).toHaveBeenCalledWith(users[1].external_user_id);
  });

  it('shows a disabled loading selector', async () => {
    const user = userEvent.setup();
    useDemoUsersQueryMock.mockReturnValue({ data: undefined, isError: false, isPending: true });

    renderSelect();

    const trigger = screen.getByRole('button', {
      name: 'Сменить аккаунт: загрузка',
    });
    expect(trigger).toHaveTextContent('Загрузка аккаунта');
    await user.click(trigger);

    expect(screen.getByRole('combobox', { hidden: true })).toBeDisabled();
    expect(screen.getByPlaceholderText('Загрузка аккаунтов…')).toBeInTheDocument();
  });

  it('shows an error when accounts cannot be loaded', async () => {
    const user = userEvent.setup();
    useDemoUsersQueryMock.mockReturnValue({ data: undefined, isError: true, isPending: false });

    renderSelect();

    await user.click(screen.getByRole('button', { name: 'Сменить аккаунт: ошибка загрузки' }));
    expect(screen.getByRole('alert', { hidden: true })).toHaveTextContent(
      'Не удалось загрузить аккаунты',
    );
    expect(screen.queryByText(/demo|backend/i)).not.toBeInTheDocument();
  });

  it('shows an empty state when there are no accounts', async () => {
    const user = userEvent.setup();
    useCurrentDemoUserMock.mockReturnValue({
      currentUser: null,
      selectUser: selectUserMock,
      userId: null,
    });
    useDemoUsersQueryMock.mockReturnValue({ data: [], isError: false, isPending: false });

    renderSelect();

    await user.click(screen.getByRole('button', { name: 'Сменить аккаунт: список пуст' }));
    expect(screen.getByText('Аккаунты не найдены')).toBeInTheDocument();
    expect(screen.queryByText(/demo|backend/i)).not.toBeInTheDocument();
  });
});
