import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { Notifications } from '@mantine/notifications';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import type { QueueAttempt } from '@/entities/queue-attempt';

const productId = '11111111-1111-4111-8111-111111111111';
const userId = '00000000-0000-4000-8000-000000000002';
const cancelQueueAttemptMock =
  jest.fn<(productId: string, userId: string) => Promise<QueueAttempt | undefined>>();
const getQueueAttemptMock =
  jest.fn<(productId: string, userId: string) => Promise<QueueAttempt | null>>();

jest.unstable_mockModule('../api/cancel-queue.api', () => ({
  cancelQueueAttempt: cancelQueueAttemptMock,
}));

const queueAttemptQueryKeys = {
  all: ['queue-attempts'],
  current: (currentProductId: string, currentUserId: string | null) => [
    'queue-attempts',
    'current',
    currentProductId,
    currentUserId,
  ],
};

jest.unstable_mockModule('@/entities/queue-attempt', () => ({
  getQueueAttempt: getQueueAttemptMock,
  queueAttemptQueryKeys,
}));

const { CancelQueueButton } = await import('./CancelQueueButton');

const waitingAttempt: QueueAttempt = {
  attempt_id: '22222222-2222-4222-8222-222222222222',
  created_at: '2026-08-07T10:00:00Z',
  message_code: 'queue_waiting',
  next_action: 'wait',
  position: 2,
  position_ahead: 1,
  product_id: productId,
  queue_sequence: 2,
  state: 'waiting',
  total_waiting: 5,
  updated_at: '2026-08-07T10:00:00Z',
};

const cancelledAttempt: QueueAttempt = {
  ...waitingAttempt,
  message_code: 'cancelled',
  next_action: 'join_queue',
  position: undefined,
  position_ahead: undefined,
  state: 'cancelled',
  terminal_at: '2026-08-07T10:01:00Z',
  total_waiting: undefined,
  updated_at: '2026-08-07T10:01:00Z',
};

const renderButton = () => {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  queryClient.setQueryData(queueAttemptQueryKeys.current(productId, userId), waitingAttempt);

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
    getQueueAttemptMock.mockReset();
    getQueueAttemptMock.mockResolvedValue(cancelledAttempt);
  });

  it('cancels once, invalidates queue queries, and replaces stale active cache from backend', async () => {
    const user = userEvent.setup();
    const queryClient = renderButton();
    const invalidateQueriesSpy = jest.spyOn(queryClient, 'invalidateQueries');

    await user.click(screen.getByRole('button', { name: 'Выйти из очереди' }));

    await waitFor(() => {
      expect(cancelQueueAttemptMock).toHaveBeenCalledWith(productId, userId);
      expect(getQueueAttemptMock).toHaveBeenCalledWith(productId, userId);
      expect(invalidateQueriesSpy).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: ['queue-attempts'] }),
      );
      expect(queryClient.getQueryData(queueAttemptQueryKeys.current(productId, userId))).toEqual(
        cancelledAttempt,
      );
    });
  });

  it('uses the attempt returned by cancellation without a follow-up request', async () => {
    const user = userEvent.setup();
    cancelQueueAttemptMock.mockResolvedValue(cancelledAttempt);
    const queryClient = renderButton();

    await user.click(screen.getByRole('button', { name: 'Выйти из очереди' }));

    await waitFor(() => {
      expect(getQueueAttemptMock).not.toHaveBeenCalled();
      expect(queryClient.getQueryData(queueAttemptQueryKeys.current(productId, userId))).toEqual(
        cancelledAttempt,
      );
    });
  });

  it('shows loading and blocks a repeated action while cancellation is pending', async () => {
    let resolveCancellation!: (attempt: QueueAttempt | undefined) => void;
    cancelQueueAttemptMock.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveCancellation = resolve;
        }),
    );
    const user = userEvent.setup();
    renderButton();
    const button = screen.getByRole('button', { name: 'Выйти из очереди' });

    await user.click(button);
    await user.click(button);

    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('data-loading', 'true');
    expect(cancelQueueAttemptMock).toHaveBeenCalledTimes(1);

    resolveCancellation(undefined);
    await waitFor(() => {
      expect(button).not.toBeDisabled();
      expect(button).not.toHaveAttribute('data-loading');
    });
  });

  it('shows a safe message when cancellation fails', async () => {
    const user = userEvent.setup();
    cancelQueueAttemptMock.mockRejectedValue(new Error('HTTP request failed: 500'));
    const queryClient = renderButton();

    await user.click(screen.getByRole('button', { name: 'Выйти из очереди' }));

    expect(await screen.findByText('Не удалось выйти из очереди')).toBeInTheDocument();
    expect(screen.queryByText(/HTTP request failed/i)).not.toBeInTheDocument();
    expect(getQueueAttemptMock).not.toHaveBeenCalled();
    expect(queryClient.getQueryData(queueAttemptQueryKeys.current(productId, userId))).toEqual(
      waitingAttempt,
    );
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
