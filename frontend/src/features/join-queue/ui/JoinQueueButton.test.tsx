import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { Notifications, notifications } from '@mantine/notifications';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import type { QueueAttempt, QueueAttemptState } from '@/entities/queue-attempt';

class ApiErrorMock extends Error {
  readonly data: unknown;
  readonly status: number;
  readonly statusText = '';

  constructor(status: number, data: unknown = undefined) {
    super(`HTTP request failed: ${status}`);
    this.data = data;
    this.status = status;
  }
}

const productId = '11111111-1111-4111-8111-111111111111';
const userId = '00000000-0000-4000-8000-000000000002';
const joinQueueMock =
  jest.fn<(productId: string, userId: string, idempotencyKey: string) => Promise<QueueAttempt>>();
const createIdempotencyKeyMock = jest.fn<() => string>();
const onJoinedMock = jest.fn<(attempt: QueueAttempt) => void>();

jest.unstable_mockModule('@/shared/api', () => ({
  ApiError: ApiErrorMock,
}));

jest.unstable_mockModule('../api/join-queue.api', () => ({
  joinQueue: joinQueueMock,
}));

jest.unstable_mockModule('../model/create-join-queue-idempotency-key', () => ({
  createJoinQueueIdempotencyKey: createIdempotencyKeyMock,
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
  queueAttemptQueryKeys,
}));

const { JoinQueueButton } = await import('./JoinQueueButton');

const makeAttempt = (state: QueueAttemptState): QueueAttempt => ({
  attempt_id: '22222222-2222-4222-8222-222222222222',
  created_at: '2026-08-07T10:00:00Z',
  message_code: state,
  next_action: 'wait',
  product_id: productId,
  queue_sequence: 2,
  state,
  updated_at: '2026-08-07T10:00:01Z',
});

const renderButton = (currentUserId: string | null = userId) => {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <MantineProvider>
        <Notifications />
        <JoinQueueButton onJoined={onJoinedMock} productId={productId} userId={currentUserId} />
      </MantineProvider>
    </QueryClientProvider>,
  );

  return queryClient;
};

describe('JoinQueueButton', () => {
  beforeEach(() => {
    notifications.clean();
    joinQueueMock.mockReset();
    createIdempotencyKeyMock.mockReset();
    createIdempotencyKeyMock.mockReturnValue('join-intention-1');
    onJoinedMock.mockReset();
  });

  it.each(['waiting', 'invited', 'checkout', 'sold_out'] as const)(
    'returns backend state %s without inventing a local transition',
    async (state) => {
      const user = userEvent.setup();
      const attempt = makeAttempt(state);
      joinQueueMock.mockResolvedValue(attempt);
      const queryClient = renderButton();
      const invalidateQueriesSpy = jest.spyOn(queryClient, 'invalidateQueries');

      await user.click(screen.getByRole('button', { name: 'Купить' }));

      await waitFor(() => {
        expect(onJoinedMock).toHaveBeenCalledWith(attempt);
      });
      expect(joinQueueMock).toHaveBeenCalledWith(productId, userId, 'join-intention-1');
      expect(invalidateQueriesSpy).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: queueAttemptQueryKeys.all }),
      );
      expect(queryClient.getQueryData(queueAttemptQueryKeys.current(productId, userId))).toEqual(
        attempt,
      );
    },
  );

  it('shows loading and blocks a repeated click while join is pending', async () => {
    let resolveJoin!: (attempt: QueueAttempt) => void;
    joinQueueMock.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveJoin = resolve;
        }),
    );
    const user = userEvent.setup();
    renderButton();
    const button = screen.getByRole('button', { name: 'Купить' });

    await user.click(button);
    await user.click(button);

    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('data-loading', 'true');
    expect(joinQueueMock).toHaveBeenCalledTimes(1);

    resolveJoin(makeAttempt('waiting'));
    await waitFor(() => {
      expect(onJoinedMock).toHaveBeenCalledWith(expect.objectContaining({ state: 'waiting' }));
    });
  });

  it('reuses the same key when the user retries after a technical error', async () => {
    const user = userEvent.setup();
    joinQueueMock
      .mockRejectedValueOnce(new Error('Failed to fetch'))
      .mockResolvedValueOnce(makeAttempt('checkout'));
    renderButton();

    await user.click(screen.getByRole('button', { name: 'Купить' }));
    expect(await screen.findByText('Не удалось войти в очередь')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Купить' }));

    await waitFor(() => {
      expect(onJoinedMock).toHaveBeenCalledWith(expect.objectContaining({ state: 'checkout' }));
    });
    expect(joinQueueMock).toHaveBeenNthCalledWith(1, productId, userId, 'join-intention-1');
    expect(joinQueueMock).toHaveBeenNthCalledWith(2, productId, userId, 'join-intention-1');
    expect(createIdempotencyKeyMock).toHaveBeenCalledTimes(1);
  });

  it('keeps queue_disabled on the product page and shows a user-facing message', async () => {
    const user = userEvent.setup();
    joinQueueMock.mockRejectedValue(new ApiErrorMock(409, { error: { code: 'queue_disabled' } }));
    renderButton();

    await user.click(screen.getByRole('button', { name: 'Купить' }));

    expect(await screen.findByText('Покупка временно недоступна')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Купить' })).toBeInTheDocument();
    expect(screen.queryByText(/HTTP request failed|409/i)).not.toBeInTheDocument();
  });

  it('explains when the waiting queue is full', async () => {
    const user = userEvent.setup();
    joinQueueMock.mockRejectedValue(new ApiErrorMock(409, { error: { code: 'queue_full' } }));
    renderButton();

    await user.click(screen.getByRole('button', { name: 'Купить' }));

    expect(await screen.findByText('Очередь заполнена')).toBeInTheDocument();
    expect(screen.getByText('Попробуйте позже или выберите другой товар.')).toBeInTheDocument();
    expect(screen.queryByText(/проверьте соединение/i)).not.toBeInTheDocument();
  });

  it.each([
    ['network', new Error('Failed to fetch')],
    ['server', new ApiErrorMock(500)],
  ])('shows a safe message for a %s error', async (_caseName, error) => {
    const user = userEvent.setup();
    joinQueueMock.mockRejectedValue(error);
    renderButton();

    await user.click(screen.getByRole('button', { name: 'Купить' }));

    expect(await screen.findByText('Не удалось войти в очередь')).toBeInTheDocument();
    expect(screen.queryByText(/Failed to fetch|HTTP request failed|500/i)).not.toBeInTheDocument();
  });

  it('is disabled until a demo user is selected', () => {
    renderButton(null);

    expect(screen.getByRole('button', { name: 'Купить' })).toBeDisabled();
  });
});
