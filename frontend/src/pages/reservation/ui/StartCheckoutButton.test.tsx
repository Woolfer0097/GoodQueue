import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { Notifications } from '@mantine/notifications';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import type { QueueAttempt } from '@/entities/queue-attempt';

const productId = '11111111-1111-4111-8111-111111111111';
const userId = '00000000-0000-4000-8000-000000000001';
const attemptId = '22222222-2222-4222-8222-222222222222';
const startCheckoutMock = jest.fn<(attemptId: string, userId: string) => Promise<QueueAttempt>>();

jest.unstable_mockModule('../api/start-checkout.api', () => ({
  startCheckout: startCheckoutMock,
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

const { StartCheckoutButton } = await import('./StartCheckoutButton');

const checkoutAttempt: QueueAttempt = {
  attempt_id: attemptId,
  checkout_started_at: '2026-08-07T10:01:00Z',
  created_at: '2026-08-07T10:00:00Z',
  deadline_at: '2026-08-07T10:06:00Z',
  message_code: 'checkout_started',
  next_action: 'complete_payment',
  product_id: productId,
  queue_sequence: 1,
  state: 'checkout',
  updated_at: '2026-08-07T10:01:00Z',
};

const renderButton = () => {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <MantineProvider>
        <Notifications />
        <StartCheckoutButton attemptId={attemptId} productId={productId} userId={userId} />
      </MantineProvider>
    </QueryClientProvider>,
  );

  return queryClient;
};

describe('StartCheckoutButton', () => {
  beforeEach(() => {
    startCheckoutMock.mockReset();
    startCheckoutMock.mockResolvedValue(checkoutAttempt);
  });

  it('starts checkout and stores only the backend-returned state for routing', async () => {
    const user = userEvent.setup();
    const queryClient = renderButton();

    await user.click(screen.getByRole('button', { name: 'Перейти к оформлению' }));

    await waitFor(() => {
      expect(startCheckoutMock).toHaveBeenCalledWith(attemptId, userId);
      expect(queryClient.getQueryData(queueAttemptQueryKeys.current(productId, userId))).toEqual(
        checkoutAttempt,
      );
    });
  });

  it('shows a safe error and does not invent a local transition', async () => {
    const user = userEvent.setup();
    startCheckoutMock.mockRejectedValue(new Error('HTTP request failed: 409'));
    const queryClient = renderButton();

    await user.click(screen.getByRole('button', { name: 'Перейти к оформлению' }));

    expect(await screen.findByText('Не удалось перейти к оформлению')).toBeInTheDocument();
    expect(
      queryClient.getQueryData(queueAttemptQueryKeys.current(productId, userId)),
    ).toBeUndefined();
    expect(screen.queryByText(/409|HTTP request failed/i)).not.toBeInTheDocument();
  });
});
