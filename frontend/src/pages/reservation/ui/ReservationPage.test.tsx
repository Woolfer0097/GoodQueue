import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';

import type { Product } from '@/entities/product';
import type { QueueAttempt, QueueAttemptState } from '@/entities/queue-attempt';

interface ProductQueryState {
  data?: Product;
}

interface QueueAttemptQueryState {
  data?: QueueAttempt | null;
  isError: boolean;
  isFetching: boolean;
  isPending: boolean;
  refetch: () => Promise<void>;
}

const productId = '11111111-1111-4111-8111-111111111111';
const userId = '00000000-0000-4000-8000-000000000001';
const useQueueAttemptQueryMock =
  jest.fn<(currentProductId: string, currentUserId: string | null) => QueueAttemptQueryState>();
const useProductQueryMock = jest.fn<(currentProductId: string) => ProductQueryState>();
const refetchMock = jest.fn<() => Promise<void>>();

jest.unstable_mockModule('@/entities/demo-user', () => ({
  useCurrentDemoUser: () => ({ userId }),
}));

jest.unstable_mockModule('@/entities/queue-attempt', () => ({
  getQueueAttemptRoute: (currentProductId: string, state: QueueAttemptState) =>
    state === 'invited'
      ? `/products/${currentProductId}/reservation`
      : state === 'checkout'
        ? `/products/${currentProductId}/checkout`
        : state === 'waiting'
          ? `/products/${currentProductId}/queue`
          : `/products/${currentProductId}/result`,
  useQueueAttemptQuery: useQueueAttemptQueryMock,
}));

jest.unstable_mockModule('@/entities/product', () => ({
  useProductQuery: useProductQueryMock,
}));

jest.unstable_mockModule('@/features/cancel-queue', () => ({
  CancelQueueButton: ({
    fullWidth,
    label,
    size,
  }: {
    fullWidth?: boolean;
    label?: string;
    size?: string;
  }) => (
    <button data-size={size} style={{ width: fullWidth ? '100%' : 'fit-content' }} type="button">
      {label}
    </button>
  ),
}));

jest.unstable_mockModule('./StartCheckoutButton', () => ({
  StartCheckoutButton: () => (
    <button data-size="md" style={{ width: '100%' }} type="button">
      Продолжить оформление
    </button>
  ),
}));

const { ReservationPage } = await import('./ReservationPage');

const createAttempt = (state: QueueAttemptState = 'invited'): QueueAttempt => ({
  attempt_id: '22222222-2222-4222-8222-222222222222',
  created_at: '2026-08-07T10:00:00Z',
  deadline_at: '2026-08-07T10:01:05Z',
  message_code: 'checkout_available',
  next_action: 'start_checkout',
  product_id: productId,
  queue_sequence: 1,
  state,
  updated_at: '2026-08-07T10:00:00Z',
});

const product: Product = {
  allocatable_stock: 1,
  category: 'Смартфоны',
  description: 'Флагманский смартфон',
  free_stock: 0,
  id: productId,
  image_url: 'https://example.com/product.jpg',
  price_cents: 1_499_000,
  queue_enabled: true,
  reserved: 1,
  title: 'Good Phone Pro',
  waiting_buffer_capacity: 100,
  waiting_count: 4,
};

const setQueryState = (state: Partial<QueueAttemptQueryState> = {}) => {
  useQueueAttemptQueryMock.mockReturnValue({
    data: createAttempt(),
    isError: false,
    isFetching: false,
    isPending: false,
    refetch: refetchMock,
    ...state,
  });
};

const PageTree = () => (
  <MantineProvider>
    <MemoryRouter initialEntries={[`/products/${productId}/reservation`]}>
      <Routes>
        <Route path="/products/:productId" element={<div>Страница товара</div>} />
        <Route path="/products/:productId/queue" element={<div>Очередь</div>} />
        <Route path="/products/:productId/reservation" element={<ReservationPage />} />
        <Route path="/products/:productId/checkout" element={<div>Оформление</div>} />
        <Route path="/products/:productId/result" element={<div>Результат</div>} />
      </Routes>
    </MemoryRouter>
  </MantineProvider>
);

describe('ReservationPage', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date('2026-08-07T10:00:00Z'));
    refetchMock.mockReset();
    refetchMock.mockResolvedValue(undefined);
    useProductQueryMock.mockReset();
    useProductQueryMock.mockReturnValue({ data: product });
    useQueueAttemptQueryMock.mockReset();
    setQueryState();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('shows the personal, time-limited reservation and required actions only for invited', () => {
    render(<PageTree />);

    expect(useQueueAttemptQueryMock).toHaveBeenCalledWith(productId, userId);
    expect(useProductQueryMock).toHaveBeenCalledWith(productId);
    expect(screen.getByRole('link', { name: product.title })).toHaveAttribute(
      'href',
      `/products/${productId}`,
    );
    expect(screen.getByRole('heading', { name: 'Товар ждёт вас' })).toBeInTheDocument();
    expect(screen.getByText(/сохранили его за вами/i)).toBeInTheDocument();
    expect(screen.getByText('01:05')).toBeInTheDocument();
    expect(screen.getByText(/резерв действует до/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Продолжить оформление' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Отказаться от резерва' })).toBeInTheDocument();
    const actions = screen.getByRole('group', { name: 'Действия резерва' });
    const checkoutButton = within(actions).getByRole('button', {
      name: 'Продолжить оформление',
    });
    const cancelButton = within(actions).getByRole('button', { name: 'Отказаться от резерва' });
    expect(actions).toHaveStyle({ flexWrap: 'wrap' });
    expect(checkoutButton.parentElement).toHaveStyle({ flex: '1 1 15rem' });
    expect(cancelButton.parentElement).toHaveStyle({ flex: '1 1 15rem' });
    expect(checkoutButton).toHaveAttribute('data-size', 'md');
    expect(cancelButton).toHaveAttribute('data-size', 'md');
    expect(checkoutButton).toHaveStyle({ width: '100%' });
    expect(cancelButton).toHaveStyle({ width: '100%' });
    expect(screen.queryByText(/персональ|backend|attempt/i)).not.toBeInTheDocument();
  });

  it('keeps content during background refetch and recalculates from the new backend deadline', () => {
    const view = render(<PageTree />);

    setQueryState({
      data: { ...createAttempt(), deadline_at: '2026-08-07T10:02:05Z' },
      isFetching: true,
    });
    view.rerender(<PageTree />);

    expect(screen.getByRole('heading', { name: 'Товар ждёт вас' })).toBeInTheDocument();
    expect(screen.getByText('02:05')).toBeInTheDocument();
    expect(screen.queryByRole('status', { name: 'Загрузка резерва' })).not.toBeInTheDocument();
  });

  it('refetches the attempt at 00:00 without locally rendering invite_expired', () => {
    setQueryState({ data: { ...createAttempt(), deadline_at: '2026-08-07T10:00:01Z' } });
    render(<PageTree />);

    act(() => {
      jest.advanceTimersByTime(1_000);
    });

    expect(screen.getByText('00:00')).toBeInTheDocument();
    expect(refetchMock).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(/время резерва истекло/i)).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Товар ждёт вас' })).toBeInTheDocument();
  });

  it('keeps the reservation usable when the exact deadline is unavailable', () => {
    const attempt = createAttempt();
    delete attempt.deadline_at;
    setQueryState({ data: attempt });

    render(<PageTree />);

    expect(screen.queryByRole('timer')).not.toBeInTheDocument();
    expect(screen.getByText(/не удалось показать точное время/i)).toBeInTheDocument();
    expect(screen.queryByText(/backend|attempt/i)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Продолжить оформление' })).toBeInTheDocument();
  });

  it('immediately refetches a deadline that is already past', () => {
    setQueryState({ data: { ...createAttempt(), deadline_at: '2026-08-07T09:59:59Z' } });
    render(<PageTree />);

    expect(screen.getByText('00:00')).toBeInTheDocument();
    expect(refetchMock).toHaveBeenCalledTimes(1);
  });

  it.each([
    ['waiting', 'Очередь'],
    ['checkout', 'Оформление'],
    ['invite_expired', 'Результат'],
    ['cancelled', 'Результат'],
  ] as const)(
    'routes direct access with backend state %s to the actual screen',
    async (state, target) => {
      setQueryState({ data: createAttempt(state) });
      render(<PageTree />);

      await waitFor(() => {
        expect(screen.getByText(target)).toBeInTheDocument();
      });
    },
  );

  it('returns direct access without an attempt to the product page', async () => {
    setQueryState({ data: null });
    render(<PageTree />);

    await waitFor(() => {
      expect(screen.getByText('Страница товара')).toBeInTheDocument();
    });
  });
});
