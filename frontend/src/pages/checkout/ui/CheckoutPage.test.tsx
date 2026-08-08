import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { act, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';

import type { Product } from '@/entities/product';
import type { QueueAttempt, QueueAttemptState } from '@/entities/queue-attempt';

interface ProductQueryState {
  data?: Product;
  isError: boolean;
  isPending: boolean;
  refetch: () => Promise<void>;
}

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
const useProductQueryMock = jest.fn<(currentProductId: string) => ProductQueryState>();
const cancelQueueButtonMock = jest.fn((_props: { label?: string }) => (
  <button type="button">{_props.label ?? 'Выйти из очереди'}</button>
));
const refetchMock = jest.fn<() => Promise<void>>();
const refetchProductMock = jest.fn<() => Promise<void>>();

jest.unstable_mockModule('@/entities/demo-user', () => ({
  useCurrentDemoUser: () => ({ userId }),
}));

jest.unstable_mockModule('@/entities/queue-attempt', () => ({
  getQueueAttemptRoute: (currentProductId: string, state: QueueAttemptState) =>
    state === 'waiting'
      ? `/products/${currentProductId}/queue`
      : state === 'invited'
        ? `/products/${currentProductId}/reservation`
        : state === 'checkout'
          ? `/products/${currentProductId}/checkout`
          : `/products/${currentProductId}/result`,
  useQueueAttemptQuery: useQueueAttemptQueryMock,
}));

jest.unstable_mockModule('@/entities/product', () => ({
  formatProductPrice: () => '14 990 ₽',
  PRODUCT_IMAGE_PLACEHOLDER: 'product-placeholder',
  useProductQuery: useProductQueryMock,
}));

jest.unstable_mockModule('@/features/cancel-queue', () => ({
  CancelQueueButton: cancelQueueButtonMock,
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
    isPending: false,
    refetch: refetchMock,
    ...state,
  });
};

const setProductQueryState = (state: Partial<ProductQueryState> = {}) => {
  useProductQueryMock.mockReturnValue({
    data: product,
    isError: false,
    isPending: false,
    refetch: refetchProductMock,
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
    jest.useFakeTimers();
    jest.setSystemTime(new Date('2026-08-07T10:05:00Z'));
    cancelQueueButtonMock.mockClear();
    refetchMock.mockReset();
    refetchMock.mockResolvedValue(undefined);
    refetchProductMock.mockReset();
    refetchProductMock.mockResolvedValue(undefined);
    useProductQueryMock.mockReset();
    useQueueAttemptQueryMock.mockReset();
    setProductQueryState();
    setQueryState();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('restores checkout from a direct URL and explains the payment limitation', () => {
    const attempt = createAttempt();
    setQueryState({ data: attempt });
    renderPage();

    expect(useProductQueryMock).toHaveBeenCalledWith(productId);
    expect(useQueueAttemptQueryMock).toHaveBeenCalledWith(productId, userId);
    expect(screen.getByRole('heading', { name: 'Товар сохранён за вами' })).toBeInTheDocument();
    expect(screen.getByText(/проверьте товар и время резерва/i)).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: product.title })).toBeInTheDocument();
    expect(screen.getByText('14 990 ₽')).toBeInTheDocument();
    expect(screen.getByRole('timer', { name: 'Осталось времени: 01:00' })).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Оплата пока недоступна в этой версии сервиса',
    );
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Завершить покупку сейчас не получится. Если вы откажетесь от покупки, товар станет доступен следующему покупателю.',
    );
    expect(screen.getByRole('alert')).not.toHaveTextContent('проверить товар');
    expect(screen.queryByText(/backend|mvp|персональ|внешн.*checkout/i)).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Перейти к оплате' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Отказаться от покупки' })).toBeInTheDocument();
    expect(cancelQueueButtonMock).toHaveBeenCalledWith(
      expect.objectContaining({
        errorTitle: 'Не удалось отказаться от покупки',
        label: 'Отказаться от покупки',
        productId,
        userId,
      }),
      undefined,
    );
  });

  it('shows the initial loading state while the queue attempt is loading', () => {
    setQueryState({ data: undefined, isPending: true });
    renderPage();

    expect(screen.getByRole('status', { name: 'Загрузка оформления' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: product.title })).not.toBeInTheDocument();
  });

  it('shows the initial loading state while the product is loading', () => {
    setProductQueryState({ data: undefined, isPending: true });
    renderPage();

    expect(screen.getByRole('status', { name: 'Загрузка оформления' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Отказаться от покупки' })).not.toBeInTheDocument();
  });

  it('updates the countdown and refetches the backend state at zero', () => {
    setQueryState({
      data: { ...createAttempt(), deadline_at: '2026-08-07T10:05:01Z' },
    });
    renderPage();

    act(() => {
      jest.advanceTimersByTime(1_000);
    });

    expect(screen.getByRole('timer', { name: 'Осталось времени: 00:00' })).toBeInTheDocument();
    expect(refetchMock).toHaveBeenCalledTimes(1);
  });

  it('keeps the checkout available when backend omits the deadline', () => {
    const attempt = createAttempt();
    delete attempt.deadline_at;
    setQueryState({ data: attempt });
    renderPage();

    expect(screen.queryByRole('timer')).not.toBeInTheDocument();
    expect(screen.getByText(/не удалось показать точное время/i)).toBeInTheDocument();
    expect(screen.queryByText(/backend|mvp|attempt/i)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Отказаться от покупки' })).toBeInTheDocument();
  });

  it('shows a retry state when the product cannot be loaded', () => {
    setProductQueryState({ data: undefined, isError: true });
    renderPage();

    expect(screen.getByRole('alert')).toHaveTextContent('Не удалось загрузить товар');
    screen.getByRole('button', { name: 'Повторить' }).click();

    expect(refetchProductMock).toHaveBeenCalledTimes(1);
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
