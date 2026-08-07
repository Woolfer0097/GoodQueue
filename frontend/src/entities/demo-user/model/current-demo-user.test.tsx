import { jest } from '@jest/globals';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

const useDemoUsersQueryMock = jest.fn();

jest.unstable_mockModule('../api/demo-user.queries', () => ({
  useDemoUsersQuery: useDemoUsersQueryMock,
}));

const { CurrentDemoUserProvider } = await import('./CurrentDemoUserProvider');
const { useCurrentDemoUser } = await import('./use-current-demo-user');

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

function CurrentDemoUserConsumer() {
  const { selectUser, userId } = useCurrentDemoUser();

  return (
    <>
      <output aria-label="current user">{userId ?? 'none'}</output>
      <button type="button" onClick={() => selectUser(users[1].external_user_id)}>
        Select second user
      </button>
    </>
  );
}

const renderProvider = () =>
  render(
    <CurrentDemoUserProvider>
      <CurrentDemoUserConsumer />
    </CurrentDemoUserProvider>,
  );

describe('current demo user state', () => {
  beforeEach(() => {
    localStorage.clear();
    useDemoUsersQueryMock.mockReset();
    useDemoUsersQueryMock.mockReturnValue({ data: users });
  });

  it('uses the first backend user as fallback and persists a new selection', async () => {
    renderProvider();

    expect(screen.getByLabelText('current user')).toHaveTextContent(users[0].external_user_id);

    fireEvent.click(screen.getByRole('button', { name: 'Select second user' }));

    expect(screen.getByLabelText('current user')).toHaveTextContent(users[1].external_user_id);
    await waitFor(() => {
      expect(localStorage.getItem('goodqueue.demo-user-id')).toBe(users[1].external_user_id);
    });
  });

  it('restores a saved user after remounting', () => {
    localStorage.setItem('goodqueue.demo-user-id', users[1].external_user_id);

    const firstRender = renderProvider();
    expect(screen.getByLabelText('current user')).toHaveTextContent(users[1].external_user_id);

    firstRender.unmount();
    renderProvider();

    expect(screen.getByLabelText('current user')).toHaveTextContent(users[1].external_user_id);
  });

  it.each([
    ['invalid', 'not-a-uuid'],
    ['stale', '00000000-0000-4000-8000-000000000099'],
  ])('replaces an %s stored value with a valid fallback', async (_caseName, storedValue) => {
    localStorage.setItem('goodqueue.demo-user-id', storedValue);

    renderProvider();

    expect(screen.getByLabelText('current user')).toHaveTextContent(users[0].external_user_id);
    await waitFor(() => {
      expect(localStorage.getItem('goodqueue.demo-user-id')).toBe(users[0].external_user_id);
    });
  });
});
