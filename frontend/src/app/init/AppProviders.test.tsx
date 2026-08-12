import { jest } from '@jest/globals';
import { notificationsStore } from '@mantine/notifications';
import { render, screen, waitFor } from '@testing-library/react';
import type { PropsWithChildren } from 'react';

const CurrentDemoUserProviderMock = jest.fn(({ children }: PropsWithChildren) => children);
const QueueAttemptNotificationsMock = jest.fn(() => null);

jest.unstable_mockModule('@/entities/demo-user', () => ({
  CurrentDemoUserProvider: CurrentDemoUserProviderMock,
  useCurrentDemoUser: () => ({ userId: null }),
}));

jest.unstable_mockModule('@/features/queue-attempt-notifications', () => ({
  QueueAttemptNotifications: QueueAttemptNotificationsMock,
}));

const { AppProviders } = await import('./AppProviders');

describe('AppProviders', () => {
  it('renders children and configures notifications', async () => {
    render(
      <AppProviders>
        <main>Application</main>
      </AppProviders>,
    );

    expect(screen.getByRole('main')).toHaveTextContent('Application');
    expect(CurrentDemoUserProviderMock).toHaveBeenCalled();
    expect(QueueAttemptNotificationsMock).toHaveBeenCalled();

    await waitFor(() => {
      expect(notificationsStore.getState()).toMatchObject({
        defaultPosition: 'top-right',
        limit: Number.POSITIVE_INFINITY,
      });
    });
  });
});
