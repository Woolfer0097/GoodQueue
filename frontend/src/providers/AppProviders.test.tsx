import { notificationsStore } from '@mantine/notifications';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';

import { AppProviders } from './AppProviders';

describe('AppProviders', () => {
  it('renders children and configures notifications', async () => {
    const queryClient = new QueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <AppProviders>
          <main>Application</main>
        </AppProviders>
      </QueryClientProvider>,
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
