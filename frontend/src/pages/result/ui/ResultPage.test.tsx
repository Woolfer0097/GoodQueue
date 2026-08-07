import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';

import type { Product } from '@/entities/product';
import type { QueueAttempt, QueueAttemptState } from '@/entities/queue-attempt';

interface QueueAttemptQueryState {
  data?: QueueAttempt | null;
  isError: boolean;
  isPending: boolean;
  refetch: () => Promise<void>;
}

interface AlternativesQueryState {
  data?: Product[];
  isError: boolean;
  isPending: boolean;
}

const productId = '11111111-1111-4111-8111-111111111111';
const userId = '00000000-0000-4000-8000-000000000002';
const refetchMock = jest.fn<() => Promise<void>>();
const useQueueAttemptQueryMock =
  jest.fn<(currentProductId: string, currentUserId: string | null) => QueueAttemptQueryState>();
const useProductAlternativesQueryMock =
  jest.fn<(currentProductId: string, enabled?: boolean) => AlternativesQueryState>();

jest.unstable_mockModule('@/entities/demo-user', () => ({
  useCurrentDemoUser: () => ({ userId }),
}));

jest.unstable_mockModule('@/entities/product', () => ({
  ProductCard: ({ product }: { product: Product }) => (
    <a href={`/products/${product.id}`}>Открыть товар: {product.title}</a>
  ),
  useProductAlternativesQuery: useProductAlternativesQueryMock,
}));

jest.unstable_mockModule('@/entities/queue-attempt', () => ({
  useQueueAttemptQuery: useQueueAttemptQueryMock,
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

const { ResultPage } = await import('./ResultPage');

const createAttempt = (state: QueueAttemptState): QueueAttempt => ({
  attempt_id: '22222222-2222-4222-8222-222222222222',
  created_at: '2026-08-07T10:00:00Z',
  message_code: state,
  next_action: state === 'purchased' ? 'none' : 'join_queue',
  product_id: productId,
  queue_sequence: 1,
  state,
  terminal_at: '2026-08-07T10:01:00Z',
  updated_at: '2026-08-07T10:01:00Z',
});

const alternative: Product = {
  allocatable_stock: 3,
  category: 'electronics',
  description: 'Доступная альтернатива',
  free_stock: 2,
  id: '33333333-3333-4333-8333-333333333333',
  image_url: 'https://example.com/alternative.jpg',
  price_cents: 99900,
  queue_enabled: true,
  reserved: 1,
  title: 'Альтернативный товар',
  waiting_buffer_capacity: 10,
  waiting_count: 0,
};

const setAttemptState = (state: Partial<QueueAttemptQueryState> = {}) => {
  useQueueAttemptQueryMock.mockReturnValue({
    data: createAttempt('purchased'),
    isError: false,
    isPending: false,
    refetch: refetchMock,
    ...state,
  });
};

const setAlternativesState = (state: Partial<AlternativesQueryState> = {}) => {
  useProductAlternativesQueryMock.mockReturnValue({
    data: undefined,
    isError: false,
    isPending: false,
    ...state,
  });
};

const renderPage = () =>
  render(
    <MantineProvider>
      <MemoryRouter initialEntries={[`/products/${productId}/result`]}>
        <Routes>
          <Route path="/" element={<div>Каталог</div>} />
          <Route path="/products/:productId" element={<div>Страница товара</div>} />
          <Route path="/products/:productId/queue" element={<div>Очередь</div>} />
          <Route path="/products/:productId/reservation" element={<div>Резерв</div>} />
          <Route path="/products/:productId/checkout" element={<div>Оформление</div>} />
          <Route path="/products/:productId/result" element={<ResultPage />} />
        </Routes>
      </MemoryRouter>
    </MantineProvider>,
  );

describe('ResultPage', () => {
  beforeEach(() => {
    refetchMock.mockReset();
    refetchMock.mockResolvedValue(undefined);
    useQueueAttemptQueryMock.mockReset();
    useProductAlternativesQueryMock.mockReset();
    setAttemptState();
    setAlternativesState();
  });

  it.each([
    ['purchased', 'Покупка подтверждена', /успешно подтверждена/i, 'Вернуться в каталог'],
    ['invite_expired', 'Время резерва истекло', /срок персонального резерва/i, 'Попробовать снова'],
    ['checkout_expired', 'Время оформления истекло', /время закончилось/i, 'Повторить покупку'],
    [
      'payment_failed',
      'Не удалось завершить покупку',
      /покупка не завершена/i,
      'Повторить покупку',
    ],
    ['cancelled', 'Вы вышли из очереди', /попытка завершена/i, 'Вернуться к товару'],
    ['sold_out', 'Товар закончился', /больше нет в наличии/i, 'Вернуться в каталог'],
  ] as const)('shows a useful result for %s', (state, heading, description, action) => {
    setAttemptState({ data: createAttempt(state) });

    renderPage();

    expect(screen.getByRole('heading', { name: heading })).toBeInTheDocument();
    expect(screen.getByText(description)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: action })).toBeInTheDocument();
    expect(screen.queryByText(new RegExp(`^${state}$`, 'i'))).not.toBeInTheDocument();
  });

  it('restores a terminal result from backend on direct URL opening', () => {
    setAttemptState({ data: createAttempt('cancelled') });

    renderPage();

    expect(useQueueAttemptQueryMock).toHaveBeenCalledWith(productId, userId);
    expect(screen.getByRole('heading', { name: 'Вы вышли из очереди' })).toBeInTheDocument();
  });

  it.each([
    ['waiting', 'Очередь'],
    ['invited', 'Резерв'],
    ['checkout', 'Оформление'],
  ] as const)('redirects active state %s to its actual route', async (state, target) => {
    setAttemptState({ data: createAttempt(state) });

    renderPage();

    await waitFor(() => expect(screen.getByText(target)).toBeInTheDocument());
  });

  it('shows a fallback with a return to the product when attempt is absent', () => {
    setAttemptState({ data: null });

    renderPage();

    expect(screen.getByRole('heading', { name: 'Результат не найден' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Вернуться к товару' })).toHaveAttribute(
      'href',
      `/products/${productId}`,
    );
  });

  it('shows alternatives for sold_out through ProductCard', () => {
    setAttemptState({ data: createAttempt('sold_out') });
    setAlternativesState({ data: [alternative] });

    renderPage();

    expect(useProductAlternativesQueryMock).toHaveBeenCalledWith(productId, true);
    expect(
      screen.getByRole('heading', { name: 'Вместо этого можно посмотреть' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: `Открыть товар: ${alternative.title}` }),
    ).toBeInTheDocument();
  });

  it('does not request alternatives for unrelated result states', () => {
    setAttemptState({ data: createAttempt('payment_failed') });

    renderPage();

    expect(useProductAlternativesQueryMock).toHaveBeenCalledWith(productId, false);
    expect(
      screen.queryByRole('heading', { name: 'Вместо этого можно посмотреть' }),
    ).not.toBeInTheDocument();
  });

  it('handles an empty alternatives list', () => {
    setAttemptState({ data: createAttempt('sold_out') });
    setAlternativesState({ data: [] });

    renderPage();

    expect(screen.getByText('Подходящих альтернатив пока нет.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Вернуться в каталог' })).toBeInTheDocument();
  });

  it('keeps the sold_out result when alternatives fail to load', () => {
    setAttemptState({ data: createAttempt('sold_out') });
    setAlternativesState({ isError: true });

    renderPage();

    expect(screen.getByRole('heading', { name: 'Товар закончился' })).toBeInTheDocument();
    expect(screen.getByText('Не удалось загрузить альтернативы')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Вернуться в каталог' })).toBeInTheDocument();
  });
});
