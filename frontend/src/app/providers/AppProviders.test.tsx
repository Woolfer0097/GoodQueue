import { notificationsStore } from '@mantine/notifications';
import { render, screen, waitFor } from '@testing-library/react';

import { AppProviders } from './AppProviders';

describe('AppProviders', () => {
  it('renders children and configures notifications', async () => {
    render(
      <AppProviders>
        <main>Application</main>
      </AppProviders>,
    );

    expect(screen.getByRole('main')).toHaveTextContent('Application');

    await waitFor(() => {
      expect(notificationsStore.getState()).toMatchObject({
        defaultPosition: 'top-right',
        limit: 3,
      });
    });
  });
});
