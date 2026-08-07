import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { Notifications, notifications } from '@mantine/notifications';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';

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
const userId = '00000000-0000-4000-8000-000000000001';
const attemptId = '22222222-2222-4222-8222-222222222222';
const checkoutMock =
  jest.fn<(currentAttemptId: string, currentUserId: string) => Promise<QueueAttempt>>();
const getQueueAttemptRouteMock = jest.fn((currentProductId: string, state: QueueAttemptState) =>
  state === 'checkout'
    ? `/products/${currentProductId}/checkout`
    : `/products/${currentProductId}/result`,
);

const queueAttemptQueryKeys = {
  all: ['queue-attempts'] as const,
  current: (currentProductId: string, currentUserId: string | null) => [
    'queue-attempts',
    'current',
    currentProductId,
    currentUserId,
  ],
};

jest.unstable_mockModule('@/shared/api', () => ({
  ApiError: ApiErrorMock,
  apiClient: jest.fn(),
}));

jest.unstable_mockModule('../api/checkout.api', () => ({
  checkout: checkoutMock,
}));

jest.unstable_mockModule('@/entities/queue-attempt', () => ({
  getQueueAttemptRoute: getQueueAttemptRouteMock,
  queueAttemptQueryKeys,
}));

const { CheckoutButton } = await import('./CheckoutButton');

const createAttempt = (state: QueueAttemptState = 'checkout'): QueueAttempt => ({
  attempt_id: attemptId,
  checkout_started_at: '2026-08-07T10:01:00Z',
  created_at: '2026-08-07T10:00:00Z',
  deadline_at: '2026-08-07T10:06:00Z',
  message_code: state === 'checkout' ? 'checkout_started' : state,
  next_action: state === 'checkout' ? 'complete_payment' : 'none',
  product_id: productId,
  queue_sequence: 1,
  state,
  updated_at: '2026-08-07T10:01:00Z',
});

function LocationProbe() {
  return <div data-testid="location">{useLocation().pathname}</div>;
}

const renderButton = (attempt: QueueAttempt | null | undefined = createAttempt()) => {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <MantineProvider>
        <Notifications />
        <MemoryRouter initialEntries={[`/products/${productId}/checkout`]}>
          <Routes>
            <Route
              path="*"
              element={
                <>
                  <CheckoutButton attempt={attempt} productId={productId} userId={userId} />
                  <LocationProbe />
                </>
              }
            />
          </Routes>
        </MemoryRouter>
      </MantineProvider>
    </QueryClientProvider>,
  );

  return queryClient;
};

describe('CheckoutButton', () => {
  beforeEach(() => {
    notifications.clean();
    checkoutMock.mockReset();
    getQueueAttemptRouteMock.mockClear();
    checkoutMock.mockResolvedValue(createAttempt('purchased'));
  });

  it('uses the current attempt, synchronizes cache and routes by backend state', async () => {
    const user = userEvent.setup();
    const queryClient = renderButton();

    await user.click(screen.getByRole('button', { name: 'Перейти к оплате' }));

    await waitFor(() => {
      expect(checkoutMock).toHaveBeenCalledWith(attemptId, userId);
      expect(queryClient.getQueryData(queueAttemptQueryKeys.current(productId, userId))).toEqual(
        createAttempt('purchased'),
      );
      expect(getQueueAttemptRouteMock).toHaveBeenCalledWith(productId, 'purchased');
      expect(screen.getByTestId('location')).toHaveTextContent(`/products/${productId}/result`);
    });
  });

  it('shows loading and blocks a repeated action while the mutation is pending', async () => {
    const user = userEvent.setup();
    let resolveCheckout!: (attempt: QueueAttempt) => void;
    const pendingCheckout = new Promise<QueueAttempt>((resolve) => {
      resolveCheckout = resolve;
    });
    checkoutMock.mockReturnValue(pendingCheckout);
    renderButton();

    const button = screen.getByRole('button', { name: 'Перейти к оплате' });
    await user.click(button);

    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('data-loading', 'true');
    await user.click(button);
    expect(checkoutMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveCheckout(createAttempt('checkout'));
      await pendingCheckout;
    });
  });

  it.each([
    ['server', new ApiErrorMock(500)],
    ['network', new Error('Failed to fetch')],
  ])('shows a safe user message for a %s error', async (_caseName, error) => {
    const user = userEvent.setup();
    checkoutMock.mockRejectedValue(error);
    renderButton();

    await user.click(screen.getByRole('button', { name: 'Перейти к оплате' }));

    expect(await screen.findByText('Не удалось завершить покупку')).toBeInTheDocument();
    expect(screen.queryByText(/Failed to fetch|HTTP request failed|500/i)).not.toBeInTheDocument();
  });

  it.each([
    ['missing', null],
    ['wrong state', createAttempt('invited')],
  ])('does not start checkout for a %s attempt', async (_caseName, attempt) => {
    const user = userEvent.setup();
    renderButton(attempt);

    const button = screen.getByRole('button', { name: 'Перейти к оплате' });
    expect(button).toBeDisabled();
    await user.click(button);
    expect(checkoutMock).not.toHaveBeenCalled();
  });
});
