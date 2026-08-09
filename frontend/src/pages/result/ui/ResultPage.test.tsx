import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';

import type { Product } from '@/entities/product';
import type { QueueAttempt, QueueAttemptState } from '@/entities/queue-attempt';

interface ProductQueryState {
  data?: Product;
}

interface QueueAttemptQueryState {
  data?: QueueAttempt | null;
  isError: boolean;
  isPending: boolean;
  refetch: () => Promise<void>;
}

const productId = '11111111-1111-4111-8111-111111111111';
const userId = '00000000-0000-4000-8000-000000000002';
const refetchMock = jest.fn<() => Promise<void>>();
const useQueueAttemptQueryMock =
  jest.fn<(currentProductId: string, currentUserId: string | null) => QueueAttemptQueryState>();
const useProductQueryMock = jest.fn<(currentProductId: string) => ProductQueryState>();
const relevantProductsMock = jest.fn<(currentProductId: string) => void>();

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
  useProductQuery: useProductQueryMock,
}));

jest.unstable_mockModule('@/features/join-queue', () => ({
  JoinQueueButton: ({ label }: { label?: string }) => <button type="button">{label}</button>,
}));

jest.unstable_mockModule('@/widgets/relevant-products', () => ({
  RelevantProducts: ({ productId: currentProductId }: { productId: string }) => {
    relevantProductsMock(currentProductId);

    return <h2>Похожие товары</h2>;
  },
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

const setAttemptState = (state: Partial<QueueAttemptQueryState> = {}) => {
  useQueueAttemptQueryMock.mockReturnValue({
    data: createAttempt('purchased'),
    isError: false,
    isPending: false,
    refetch: refetchMock,
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
    useProductQueryMock.mockReset();
    useProductQueryMock.mockReturnValue({ data: product });
    useQueueAttemptQueryMock.mockReset();
    relevantProductsMock.mockReset();
    setAttemptState();
  });

  it.each([
    [
      'purchased',
      'Покупка завершена',
      'Если товар ещё в наличии, вы можете купить ещё один.',
      'Купить ещё',
      'link',
      `/products/${productId}`,
    ],
    [
      'invite_expired',
      'Время резерва истекло',
      /больше не можем держать товар/i,
      'Попробовать снова',
      'link',
      `/products/${productId}`,
    ],
    [
      'checkout_expired',
      'Время оформления истекло',
      /резерв закончился/i,
      'Повторить покупку',
      'button',
      null,
    ],
    [
      'payment_failed',
      'Оплата не прошла',
      /попробуйте ещё раз/i,
      'Повторить покупку',
      'button',
      null,
    ],
    [
      'cancelled',
      'Покупка отменена',
      /начать снова/i,
      'Вернуться к товару',
      'link',
      `/products/${productId}`,
    ],
    ['sold_out', 'Товар закончился', /больше нет в наличии/i, 'Вернуться в каталог', 'link', '/'],
  ] as const)(
    'shows a useful result for %s',
    (state, heading, description, action, actionRole, actionPath) => {
      setAttemptState({ data: createAttempt(state) });

      renderPage();

      expect(screen.getByRole('heading', { name: heading })).toBeInTheDocument();
      expect(screen.getByText(description)).toBeInTheDocument();
      expect(screen.getByRole(actionRole, { name: action }).getAttribute('href')).toBe(actionPath);
      expect(screen.queryByText(new RegExp(`^${state}$`, 'i'))).not.toBeInTheDocument();
      expect(screen.queryByText(/попытк|персональ|backend|attempt|mvp/i)).not.toBeInTheDocument();
    },
  );

  it.each([
    ['purchased', 'Покупка завершена'],
    ['payment_failed', 'Оплата не прошла'],
    ['checkout_expired', 'Время оформления истекло'],
  ] as const)('restores %s from backend on direct URL opening', (state, heading) => {
    setAttemptState({ data: createAttempt(state) });

    renderPage();

    expect(useQueueAttemptQueryMock).toHaveBeenCalledWith(productId, userId);
    expect(screen.getByRole('heading', { name: heading })).toBeInTheDocument();
  });

  it.each([
    'purchased',
    'cancelled',
    'invite_expired',
    'checkout_expired',
    'payment_failed',
  ] as const)('offers the catalog as a secondary exit for %s', (state) => {
    setAttemptState({ data: createAttempt(state) });

    renderPage();

    expect(screen.getByRole('link', { name: 'Вернуться в каталог' })).toHaveAttribute('href', '/');
    expect(screen.getByRole('link', { name: 'Вернуться в каталог' })).toHaveAttribute(
      'data-size',
      'md',
    );
    expect(screen.getByRole('link', { name: 'Вернуться в каталог' })).toHaveAttribute(
      'data-variant',
      'light',
    );
    const actions = screen.getByRole('group', { name: 'Действия покупки' });
    expect(within(actions).getByRole('link', { name: 'Вернуться в каталог' })).toBe(
      screen.getByRole('link', { name: 'Вернуться в каталог' }),
    );
  });

  it('explains both available exits after cancellation', () => {
    setAttemptState({ data: createAttempt('cancelled') });

    renderPage();

    expect(
      screen.getByText('Вы можете вернуться к товару и начать снова или перейти в каталог.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Вернуться к товару' })).toHaveAttribute(
      'data-size',
      'md',
    );
    expect(useProductQueryMock).toHaveBeenCalledWith(productId);
    expect(screen.getByRole('link', { name: product.title })).toHaveAttribute(
      'href',
      `/products/${productId}`,
    );
    const actions = screen.getByRole('group', { name: 'Действия покупки' });
    expect(within(actions).getByRole('link', { name: 'Вернуться к товару' })).toBeInTheDocument();
    expect(within(actions).getByRole('link', { name: 'Вернуться в каталог' })).toBeInTheDocument();
  });

  it.each(['sold_out', 'payment_failed', 'checkout_expired', 'cancelled'] as const)(
    'shows similar products for the recoverable result %s',
    (state) => {
      setAttemptState({ data: createAttempt(state) });

      renderPage();

      expect(relevantProductsMock).toHaveBeenCalledWith(productId);
      expect(screen.getByRole('heading', { name: 'Похожие товары' })).toBeInTheDocument();
    },
  );

  it.each(['purchased', 'invite_expired'] as const)(
    'does not show similar products for %s',
    (state) => {
      setAttemptState({ data: createAttempt(state) });

      renderPage();

      expect(relevantProductsMock).not.toHaveBeenCalled();
      expect(screen.queryByRole('heading', { name: 'Похожие товары' })).not.toBeInTheDocument();
    },
  );

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

    expect(screen.getByRole('heading', { name: 'Покупка не найдена' })).toBeInTheDocument();
    expect(screen.getByText('Для этого товара нет завершённой покупки.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Вернуться к товару' })).toHaveAttribute(
      'href',
      `/products/${productId}`,
    );
  });
});
