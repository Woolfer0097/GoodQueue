import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { Notifications } from '@mantine/notifications';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const productId = '11111111-1111-4111-8111-111111111111';
const userId = '00000000-0000-4000-8000-000000000002';
const cancelQueueAttemptMock = jest.fn<(productId: string, userId: string) => Promise<void>>();

jest.unstable_mockModule('../api/cancel-queue.api', () => ({
  cancelQueueAttempt: cancelQueueAttemptMock,
}));

const queueAttemptQueryKeys = {
  current: (currentProductId: string, currentUserId: string | null) => [
    'queue-attempts',
    'current',
    currentProductId,
    currentUserId,
  ],
};

jest.unstable_mockModule('@/entities/queue-attempt', () => ({
  queueAttemptQueryKeys,
}));

const { CancelQueueButton } = await import('./CancelQueueButton');

const renderButton = () => {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  queryClient.setQueryData(queueAttemptQueryKeys.current(productId, userId), {
    state: 'waiting',
  });

  render(
    <QueryClientProvider client={queryClient}>
      <MantineProvider>
        <Notifications />
        <CancelQueueButton productId={productId} userId={userId} />
      </MantineProvider>
    </QueryClientProvider>,
  );

  return queryClient;
};

describe('CancelQueueButton', () => {
  beforeEach(() => {
    cancelQueueAttemptMock.mockReset();
    cancelQueueAttemptMock.mockResolvedValue(undefined);
  });

  it('cancels once and invalidates the backend attempt query', async () => {
    const user = userEvent.setup();
    const queryClient = renderButton();

    await user.click(screen.getByRole('button', { name: 'Выйти из очереди' }));

    await waitFor(() => {
      expect(cancelQueueAttemptMock).toHaveBeenCalledWith(productId, userId);
      expect(queryClient.getQueryState(queueAttemptQueryKeys.current(productId, userId))).toEqual(
        expect.objectContaining({ isInvalidated: true }),
      );
    });
  });

  it('shows a safe message when cancellation fails', async () => {
    const user = userEvent.setup();
    cancelQueueAttemptMock.mockRejectedValue(new Error('HTTP request failed: 500'));
    renderButton();

    await user.click(screen.getByRole('button', { name: 'Выйти из очереди' }));

    expect(await screen.findByText('Не удалось выйти из очереди')).toBeInTheDocument();
    expect(screen.queryByText(/HTTP request failed/i)).not.toBeInTheDocument();
  });

  it('is unavailable until the demo user is known', () => {
    const queryClient = new QueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MantineProvider>
          <CancelQueueButton productId={productId} userId={null} />
        </MantineProvider>
      </QueryClientProvider>,
    );

    expect(screen.getByRole('button', { name: 'Выйти из очереди' })).toBeDisabled();
  });
});
