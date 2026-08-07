import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';

import type { QueueAttempt, QueueAttemptState } from '@/entities/queue-attempt';

interface QueueAttemptQueryState {
  data?: QueueAttempt | null;
  isError: boolean;
  isPending: boolean;
  refetch: () => Promise<void>;
}

const productId = '11111111-1111-4111-8111-111111111111';
const userId = '00000000-0000-4000-8000-000000000001';
const useQueueAttemptQueryMock =
  jest.fn<(currentProductId: string, currentUserId: string | null) => QueueAttemptQueryState>();
const checkoutButtonMock = jest.fn(({ attempt }: { attempt: QueueAttempt }) => (
  <button type="button">Оплатить attempt {attempt.attempt_id}</button>
));
const refetchMock = jest.fn<() => Promise<void>>();

jest.unstable_mockModule('@/entities/demo-user', () => ({
  useCurrentDemoUser: () => ({ userId }),
}));

jest.unstable_mockModule('@/entities/queue-attempt', () => ({
  useQueueAttemptQuery: useQueueAttemptQueryMock,
}));

jest.unstable_mockModule('@/features/checkout', () => ({
  CheckoutButton: checkoutButtonMock,
}));

jest.unstable_mockModule('@/features/queue-polling', () => ({
  getQueueAttemptRoute: (currentProductId: string, state: QueueAttemptState) =>
    state === 'waiting'
      ? `/products/${currentProductId}/queue`
      : state === 'invited'
        ? `/products/${currentProductId}/reservation`
        : state === 'checkout'
          ? `/products/${currentProductId}/checkout`
          : `/products/${currentProductId}/result`,
}));

const { CheckoutPage } = await import('./CheckoutPage');

const createAttempt = (state: QueueAttemptState = 'checkout'): QueueAttempt => ({
  attempt_id: '22222222-2222-4222-8222-222222222222',
  checkout_started_at: '2026-08-07T10:01:00Z',
  created_at: '2026-08-07T10:00:00Z',
  deadline_at: '2026-08-07T10:06:00Z',
  message_code: 'checkout_started',
  next_action: 'complete_payment',
  product_id: productId,
  queue_sequence: 1,
  state,
  updated_at: '2026-08-07T10:01:00Z',
});

const setQueryState = (state: Partial<QueueAttemptQueryState> = {}) => {
  useQueueAttemptQueryMock.mockReturnValue({
    data: createAttempt(),
    isError: false,
    isPending: false,
    refetch: refetchMock,
    ...state,
  });
};

const renderPage = () =>
  render(
    <MantineProvider>
      <MemoryRouter initialEntries={[`/products/${productId}/checkout`]}>
        <Routes>
          <Route path="/products/:productId" element={<div>Страница товара</div>} />
          <Route path="/products/:productId/queue" element={<div>Очередь</div>} />
          <Route path="/products/:productId/reservation" element={<div>Резерв</div>} />
          <Route path="/products/:productId/checkout" element={<CheckoutPage />} />
          <Route path="/products/:productId/result" element={<div>Результат</div>} />
        </Routes>
      </MemoryRouter>
    </MantineProvider>,
  );

describe('CheckoutPage', () => {
  beforeEach(() => {
    checkoutButtonMock.mockClear();
    refetchMock.mockReset();
    refetchMock.mockResolvedValue(undefined);
    useQueueAttemptQueryMock.mockReset();
    setQueryState();
  });

  it('shows the personal purchase right and passes the current attempt to payment imitation', () => {
    const attempt = createAttempt();
    setQueryState({ data: attempt });
    renderPage();

    expect(useQueueAttemptQueryMock).toHaveBeenCalledWith(productId, userId);
    expect(
      screen.getByRole('heading', { name: 'Ваше право на покупку подтверждено' }),
    ).toBeInTheDocument();
    expect(screen.getByText(/персональный временный доступ/i)).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: `Оплатить attempt ${attempt.attempt_id}` }),
    ).toBeInTheDocument();
    expect(checkoutButtonMock).toHaveBeenCalledWith(
      expect.objectContaining({ attempt, productId, userId }),
      undefined,
    );
  });

  it.each([
    ['waiting', 'Очередь'],
    ['invited', 'Резерв'],
    ['purchased', 'Результат'],
    ['payment_failed', 'Результат'],
  ] as const)('routes backend state %s through the shared rule', async (state, target) => {
    setQueryState({ data: createAttempt(state) });
    renderPage();

    await waitFor(() => expect(screen.getByText(target)).toBeInTheDocument());
  });

  it('returns to the product when there is no current attempt', async () => {
    setQueryState({ data: null });
    renderPage();

    await waitFor(() => expect(screen.getByText('Страница товара')).toBeInTheDocument());
  });
});
