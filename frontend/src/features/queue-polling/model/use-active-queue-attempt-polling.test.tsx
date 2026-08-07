import { jest } from '@jest/globals';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router';

import type { QueueAttempt, QueueAttemptState } from '@/entities/queue-attempt';

const getQueueAttemptMock =
  jest.fn<(productId: string, userId: string) => Promise<QueueAttempt | null>>();

jest.unstable_mockModule('@/entities/queue-attempt', () => ({
  getQueueAttempt: getQueueAttemptMock,
  getQueueAttemptRoute: (currentProductId: string, state: QueueAttemptState) =>
    state === 'waiting'
      ? `/products/${currentProductId}/queue`
      : state === 'invited'
        ? `/products/${currentProductId}/reservation`
        : state === 'checkout'
          ? `/products/${currentProductId}/checkout`
          : `/products/${currentProductId}/result`,
  queueAttemptQueryKeys: {
    current: (productId: string, userId: string | null) => [
      'queue-attempts',
      'current',
      productId,
      userId,
    ],
  },
}));

const { QUEUE_ATTEMPT_POLLING_INTERVAL_MS, useActiveQueueAttemptPolling } =
  await import('./use-active-queue-attempt-polling');

const productId = '11111111-1111-4111-8111-111111111111';
const firstUserId = '00000000-0000-4000-8000-000000000001';
const secondUserId = '00000000-0000-4000-8000-000000000002';

const createAttempt = (state: QueueAttemptState): QueueAttempt => ({
  attempt_id: '22222222-2222-4222-8222-222222222222',
  created_at: '2026-08-07T10:00:00Z',
  message_code: `attempt_${state}`,
  next_action: 'wait',
  product_id: productId,
  queue_sequence: 1,
  state,
  updated_at: '2026-08-07T10:00:00Z',
});

function PollingHarness({
  currentProductId = productId,
  userId,
}: {
  currentProductId?: string;
  userId: string | null;
}) {
  const query = useActiveQueueAttemptPolling({ productId: currentProductId, userId });
  const location = useLocation();

  return (
    <>
      <div data-testid="attempt-state">{query.data?.state ?? 'empty'}</div>
      <div data-testid="fetch-status">{query.isFetching ? 'fetching' : 'idle'}</div>
      <div data-testid="location">{location.pathname}</div>
    </>
  );
}

const renderPolling = (userId: string | null, currentProductId = productId) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/products/${productId}/queue`]}>
        <PollingHarness currentProductId={currentProductId} userId={userId} />
      </MemoryRouter>
    </QueryClientProvider>,
  );

  return {
    ...view,
    rerenderUser: (nextUserId: string | null) =>
      view.rerender(
        <QueryClientProvider client={queryClient}>
          <MemoryRouter initialEntries={[`/products/${productId}/queue`]}>
            <PollingHarness currentProductId={currentProductId} userId={nextUserId} />
          </MemoryRouter>
        </QueryClientProvider>,
      ),
  };
};

describe('useActiveQueueAttemptPolling', () => {
  beforeEach(() => {
    getQueueAttemptMock.mockReset();
  });

  it('polls an active attempt and stops after a terminal response', async () => {
    getQueueAttemptMock
      .mockResolvedValueOnce(createAttempt('waiting'))
      .mockResolvedValueOnce(createAttempt('purchased'));

    renderPolling(firstUserId);

    await screen.findByText('waiting');
    expect(getQueueAttemptMock).toHaveBeenCalledTimes(1);

    await waitFor(
      () => {
        expect(screen.getByTestId('attempt-state')).toHaveTextContent('purchased');
      },
      {
        timeout: QUEUE_ATTEMPT_POLLING_INTERVAL_MS + 1_000,
      },
    );

    expect(getQueueAttemptMock).toHaveBeenCalledTimes(2);
    expect(screen.getByTestId('location')).toHaveTextContent(`/products/${productId}/result`);

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, QUEUE_ATTEMPT_POLLING_INTERVAL_MS + 250));
    });

    expect(getQueueAttemptMock).toHaveBeenCalledTimes(2);
  });

  it('routes waiting to invited and invited to checkout as backend state changes', async () => {
    getQueueAttemptMock
      .mockResolvedValueOnce(createAttempt('waiting'))
      .mockResolvedValueOnce(createAttempt('invited'))
      .mockResolvedValueOnce(createAttempt('checkout'));

    renderPolling(firstUserId);
    await screen.findByText('waiting');

    expect(screen.getByTestId('location')).toHaveTextContent(`/products/${productId}/queue`);

    await waitFor(
      () => {
        expect(screen.getByTestId('location')).toHaveTextContent(
          `/products/${productId}/reservation`,
        );
      },
      {
        timeout: QUEUE_ATTEMPT_POLLING_INTERVAL_MS + 1_000,
      },
    );

    await waitFor(
      () => {
        expect(screen.getByTestId('location')).toHaveTextContent(`/products/${productId}/checkout`);
      },
      {
        timeout: QUEUE_ATTEMPT_POLLING_INTERVAL_MS + 1_000,
      },
    );
  });

  it('drops the previous user attempt immediately when demo user changes', async () => {
    let resolveSecondUser!: (attempt: QueueAttempt) => void;
    const secondUserRequest = new Promise<QueueAttempt>((resolve) => {
      resolveSecondUser = resolve;
    });
    getQueueAttemptMock
      .mockResolvedValueOnce(createAttempt('waiting'))
      .mockReturnValueOnce(secondUserRequest);

    const { rerenderUser } = renderPolling(firstUserId);
    await screen.findByText('waiting');

    rerenderUser(secondUserId);

    expect(screen.getByTestId('attempt-state')).toHaveTextContent('empty');
    expect(getQueueAttemptMock).toHaveBeenLastCalledWith(productId, secondUserId);

    await act(async () => {
      resolveSecondUser(createAttempt('invited'));
      await secondUserRequest;
    });
    await screen.findByText('invited');
  });

  it('does not request an attempt without both identifiers', async () => {
    getQueueAttemptMock.mockResolvedValue(null);
    const { rerenderUser } = renderPolling(null);
    await act(async () => {
      await Promise.resolve();
    });

    expect(getQueueAttemptMock).not.toHaveBeenCalled();

    rerenderUser(firstUserId);
    await waitFor(() => {
      expect(getQueueAttemptMock).toHaveBeenCalledTimes(1);
    });
  });

  it('keeps current data visible during a background refetch', async () => {
    let resolveRefetch!: (attempt: QueueAttempt) => void;
    const refetchRequest = new Promise<QueueAttempt>((resolve) => {
      resolveRefetch = resolve;
    });
    getQueueAttemptMock
      .mockResolvedValueOnce(createAttempt('waiting'))
      .mockReturnValueOnce(refetchRequest);

    renderPolling(firstUserId);
    await screen.findByText('waiting');

    await waitFor(
      () => {
        expect(screen.getByTestId('fetch-status')).toHaveTextContent('fetching');
      },
      {
        timeout: QUEUE_ATTEMPT_POLLING_INTERVAL_MS + 1_000,
      },
    );

    expect(screen.getByTestId('attempt-state')).toHaveTextContent('waiting');

    await act(async () => {
      resolveRefetch(createAttempt('waiting'));
      await refetchRequest;
    });
  });
});
