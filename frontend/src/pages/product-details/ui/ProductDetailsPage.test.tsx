import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router';

import type { Product } from '@/entities/product';
import type { QueueAttempt } from '@/entities/queue-attempt';

interface ProductQueryState {
  data?: Product;
  error?: unknown;
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

const useProductQueryMock = jest.fn<(productId: string) => ProductQueryState>();
const refetchMock = jest.fn<() => Promise<void>>();
const joinQueueCtaMock = jest.fn<(productId: string, userId: string | null) => void>();
const useQueueAttemptQueryMock =
  jest.fn<(productId: string, userId: string | null) => QueueAttemptQueryState>();
const refetchQueueAttemptMock = jest.fn<() => Promise<void>>();
const relevantProductsMock = jest.fn<(productId: string) => void>();
const userId = '00000000-0000-4000-8000-000000000002';
const waitingAttempt: QueueAttempt = {
  attempt_id: '22222222-2222-4222-8222-222222222222',
  created_at: '2026-08-07T10:00:00Z',
  message_code: 'queue_waiting',
  next_action: 'wait',
  product_id: '11111111-1111-1111-1111-111111111111',
  queue_sequence: 2,
  state: 'waiting',
  updated_at: '2026-08-07T10:00:01Z',
};

jest.unstable_mockModule('@/entities/product', () => ({
  formatProductCategory: () => 'Коллекционирование',
  formatProductPrice: () => '14 990 ₽',
  ProductAvailabilityBadge: ({ product: currentProduct }: { product: Product }) => (
    <span>{currentProduct.free_stock > 0 ? 'В наличии' : 'Доступно по очереди'}</span>
  ),
  PRODUCT_IMAGE_PLACEHOLDER: 'data:image/svg+xml,placeholder',
  useProductQuery: useProductQueryMock,
}));

jest.unstable_mockModule('@/entities/queue-attempt', () => ({
  getQueueAttemptRoute: (currentProductId: string, state: QueueAttempt['state']) =>
    ({
      cancelled: `/products/${currentProductId}/result`,
      checkout: `/products/${currentProductId}/checkout`,
      checkout_expired: `/products/${currentProductId}/result`,
      invite_expired: `/products/${currentProductId}/result`,
      invited: `/products/${currentProductId}/reservation`,
      payment_failed: `/products/${currentProductId}/result`,
      purchased: `/products/${currentProductId}/result`,
      sold_out: `/products/${currentProductId}/result`,
      waiting: `/products/${currentProductId}/queue`,
    })[state],
  useQueueAttemptQuery: useQueueAttemptQueryMock,
}));

jest.unstable_mockModule('@/features/join-queue', () => ({
  JoinQueueButton: ({
    label = 'Купить',
    onJoined,
    productId,
    userId: currentUserId,
  }: {
    label?: string;
    onJoined: (attempt: QueueAttempt) => void;
    productId: string;
    userId: string;
  }) => (
    <button
      onClick={() => {
        joinQueueCtaMock(productId, currentUserId);
        onJoined(waitingAttempt);
      }}
    >
      {label}
    </button>
  ),
}));

jest.unstable_mockModule('@/features/select-demo-user', () => ({
  useCurrentDemoUser: () => ({ userId }),
}));

jest.unstable_mockModule('@/widgets/relevant-products', () => ({
  RelevantProducts: ({ productId }: { productId: string }) => {
    relevantProductsMock(productId);

    return <h2>Похожие товары</h2>;
  },
}));

const { ProductDetailsPage } = await import('./ProductDetailsPage');

const product: Product = {
  allocatable_stock: 3,
  category: 'collectibles',
  description: 'Внутреннее описание товара',
  free_stock: 2,
  id: '11111111-1111-1111-1111-111111111111',
  image_url: 'https://example.com/product.jpg',
  price_cents: 1499000,
  queue_enabled: true,
  reserved: 1,
  title: 'Лимитированный товар',
  waiting_buffer_capacity: 3,
  waiting_count: 2,
};

const setQueryState = (state: Partial<ProductQueryState> = {}) => {
  useProductQueryMock.mockReturnValue({
    data: product,
    isError: false,
    isPending: false,
    refetch: refetchMock,
    ...state,
  });
};

const setQueueAttemptQueryState = (state: Partial<QueueAttemptQueryState> = {}) => {
  useQueueAttemptQueryMock.mockReturnValue({
    data: null,
    isError: false,
    isPending: false,
    refetch: refetchQueueAttemptMock,
    ...state,
  });
};

const renderPage = (initialEntry = `/products/${product.id}`) =>
  render(
    <MantineProvider>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/" element={<div>Каталог</div>} />
          <Route path="/products/:productId" element={<ProductDetailsPage />} />
          <Route path="/products/:productId/queue" element={<div>Очередь</div>} />
        </Routes>
      </MemoryRouter>
    </MantineProvider>,
  );

describe('ProductDetailsPage', () => {
  beforeEach(() => {
    refetchMock.mockReset();
    refetchMock.mockResolvedValue(undefined);
    useProductQueryMock.mockReset();
    useQueueAttemptQueryMock.mockReset();
    joinQueueCtaMock.mockReset();
    relevantProductsMock.mockReset();
    refetchQueueAttemptMock.mockReset();
    refetchQueueAttemptMock.mockResolvedValue(undefined);
    setQueryState();
    setQueueAttemptQueryState();
  });

  it('loads the route product and shows only user-facing details in mobile reading order', () => {
    renderPage();

    expect(useProductQueryMock).toHaveBeenCalledWith(product.id);

    const image = screen.getByRole('img', { name: product.title });
    const price = screen.getByText('14 990 ₽');
    const title = screen.getByRole('heading', { name: product.title });
    const availability = screen.getByText('В наличии');
    const stock = screen.getByText('Осталось: 2');

    expect(image.compareDocumentPosition(price) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(price.compareDocumentPosition(title) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(
      title.compareDocumentPosition(availability) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      availability.compareDocumentPosition(stock) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(screen.queryByText(/reserved/i)).not.toBeInTheDocument();
    expect(screen.queryByText(product.id)).not.toBeInTheDocument();
    expect(screen.getByText(product.description)).toBeInTheDocument();
    expect(screen.getByText('Коллекционирование')).toBeInTheDocument();
    expect(screen.queryByText(/^В очереди:/)).not.toBeInTheDocument();
    expect(screen.queryByText(/распределени/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/лимит очереди/i)).not.toBeInTheDocument();
    const buyButton = screen.getByRole('button', { name: 'Купить' });
    expect(
      stock.compareDocumentPosition(buyButton) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it('shows similar products after the primary purchase action', () => {
    renderPage();

    const buyButton = screen.getByRole('button', { name: 'Купить' });
    const relevantProductsHeading = screen.getByRole('heading', { name: 'Похожие товары' });

    expect(relevantProductsMock).toHaveBeenCalledWith(product.id);
    expect(
      buyButton.compareDocumentPosition(relevantProductsHeading) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it('replaces buy with the current queue state and route', () => {
    setQueueAttemptQueryState({ data: waitingAttempt });

    renderPage();

    expect(screen.getByText('Вы уже в очереди')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Вернуться в очередь' })).toHaveAttribute(
      'href',
      `/products/${product.id}/queue`,
    );
    expect(screen.queryByRole('button', { name: 'Купить' })).not.toBeInTheDocument();
  });

  it.each([
    ['invited', 'Товар ждёт вас', 'Продолжить оформление', 'reservation'],
    ['checkout', 'Товар сохранён за вами', 'Продолжить оформление', 'checkout'],
    ['purchased', 'Покупка завершена', 'Посмотреть результат', 'result'],
  ] as const)(
    'shows a single next action for %s',
    (state, statusLabel, actionLabel, routeSegment) => {
      setQueueAttemptQueryState({ data: { ...waitingAttempt, state } });

      renderPage();

      expect(screen.getByText(statusLabel)).toBeInTheDocument();
      expect(screen.getByRole('link', { name: actionLabel })).toHaveAttribute(
        'href',
        `/products/${product.id}/${routeSegment}`,
      );
      expect(screen.queryByRole('button', { name: 'Купить' })).not.toBeInTheDocument();
    },
  );

  it('offers the current purchase action after a recoverable terminal state', () => {
    setQueueAttemptQueryState({ data: { ...waitingAttempt, state: 'cancelled' } });

    renderPage();

    expect(screen.getByText('Покупка отменена')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Купить' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Попробовать снова' })).not.toBeInTheDocument();
  });

  it('offers the queue after a recoverable terminal state when no item is free', () => {
    setQueryState({ data: { ...product, free_stock: 0 } });
    setQueueAttemptQueryState({ data: { ...waitingAttempt, state: 'checkout_expired' } });

    renderPage();

    expect(screen.getByText('Время оформления истекло')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Встать в очередь' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Купить' })).not.toBeInTheDocument();
  });

  it.each([
    ['invite_expired', 'Время резерва истекло'],
    ['sold_out', 'Товар закончился'],
  ] as const)('keeps the %s explanation free of internal purchase terms', (state, status) => {
    setQueueAttemptQueryState({ data: { ...waitingAttempt, state } });

    renderPage();

    expect(screen.getByText(status)).toBeInTheDocument();
    expect(screen.queryByText(/персональ|экран результата|попытк/i)).not.toBeInTheDocument();
  });

  it('does not offer a new attempt after a recoverable state when the queue is full', () => {
    setQueryState({
      data: {
        ...product,
        free_stock: 0,
        waiting_buffer_capacity: 2,
        waiting_count: 2,
      },
    });
    setQueueAttemptQueryState({ data: { ...waitingAttempt, state: 'payment_failed' } });

    renderPage();

    expect(screen.getByText('Оплата не прошла')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Очередь заполнена' })).toBeDisabled();
    expect(screen.queryByRole('button', { name: 'Встать в очередь' })).not.toBeInTheDocument();
  });

  it('uses a queue-specific action when no item is free', () => {
    setQueryState({ data: { ...product, free_stock: 0 } });

    renderPage();

    expect(screen.getByText('Доступно по очереди')).toBeInTheDocument();
    expect(screen.queryByText('Осталось: 0')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Встать в очередь' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Купить' })).not.toBeInTheDocument();
  });

  it('does not offer a join when the waiting queue is already full', () => {
    setQueryState({
      data: {
        ...product,
        free_stock: 0,
        waiting_buffer_capacity: 2,
        waiting_count: 2,
      },
    });

    renderPage();

    expect(screen.getByRole('button', { name: 'Очередь заполнена' })).toBeDisabled();
    expect(screen.getByText('Попробуйте позже или выберите другой товар.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Встать в очередь' })).not.toBeInTheDocument();
  });

  it('does not offer a purchase when the queue is disabled', () => {
    setQueryState({ data: { ...product, queue_enabled: false } });

    renderPage();

    expect(screen.getByRole('button', { name: 'Покупка недоступна' })).toBeDisabled();
    expect(screen.queryByRole('button', { name: 'Купить' })).not.toBeInTheDocument();
  });

  it('does not flash buy while the current attempt is loading', () => {
    setQueueAttemptQueryState({ data: undefined, isPending: true });

    renderPage();

    expect(screen.getByRole('button', { name: 'Проверяем доступность' })).toBeDisabled();
    expect(screen.queryByRole('button', { name: 'Купить' })).not.toBeInTheDocument();
  });

  it('does not offer a new purchase when the current attempt cannot be checked', async () => {
    const user = userEvent.setup();
    setQueueAttemptQueryState({ data: undefined, isError: true });

    renderPage();

    expect(screen.getByRole('alert')).toHaveTextContent('Не удалось проверить статус покупки');
    expect(screen.queryByRole('button', { name: 'Купить' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Проверить ещё раз' }));

    expect(refetchQueueAttemptMock).toHaveBeenCalledTimes(1);
  });

  it('passes the loaded product and selected account to join-queue', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole('button', { name: 'Купить' }));

    expect(joinQueueCtaMock).toHaveBeenCalledWith(product.id, userId);
    expect(screen.getByText('Очередь')).toBeInTheDocument();
  });

  it('shows Skeleton placeholders during the first load', () => {
    setQueryState({ data: undefined, isPending: true });

    renderPage();

    expect(screen.getByRole('status', { name: 'Загрузка товара' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: product.title })).not.toBeInTheDocument();
  });

  it('hides a zero free-stock counter and internal queue metrics', () => {
    setQueryState({ data: { ...product, free_stock: 0, waiting_count: 0 } });

    renderPage();

    expect(screen.getByText('Доступно по очереди')).toBeInTheDocument();
    expect(screen.queryByText('Осталось: 0')).not.toBeInTheDocument();
    expect(screen.queryByText(/^В очереди:/)).not.toBeInTheDocument();
  });

  it('shows a safe server error and retries the request', async () => {
    const user = userEvent.setup();
    setQueryState({
      data: undefined,
      error: new Error('HTTP request failed: 500 Internal Server Error'),
      isError: true,
    });

    renderPage();

    expect(screen.getByRole('alert')).toHaveTextContent('Не удалось загрузить товар');
    expect(screen.queryByText(/HTTP request failed/i)).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Повторить' }));

    expect(refetchMock).toHaveBeenCalledTimes(1);
  });

  it('shows a dedicated not-found state for an unknown product', () => {
    setQueryState({
      data: undefined,
      error: { data: { error: { code: 'not_found' } }, status: 404 },
      isError: true,
    });

    renderPage('/products/99999999-9999-4999-8999-999999999999');

    expect(screen.getByRole('heading', { name: 'Товар не найден' })).toBeInTheDocument();
    expect(screen.queryByText('Не удалось загрузить товар')).not.toBeInTheDocument();
  });

  it('shows the product path and returns to the catalog with breadcrumbs', async () => {
    const user = userEvent.setup();
    renderPage();

    const breadcrumbs = screen.getByRole('navigation', { name: 'Хлебные крошки' });
    const catalogLink = screen.getByRole('link', { name: 'Каталог' });
    expect(catalogLink).toHaveAttribute('href', '/');
    expect(breadcrumbs).not.toHaveTextContent('/');
    expect(
      screen.getByText(product.title, { selector: '[aria-current="page"]' }),
    ).toBeInTheDocument();

    await user.click(catalogLink);

    expect(screen.getByText('Каталог')).toBeInTheDocument();
  });

  it('explains when a direct queue URL has no active attempt', () => {
    render(
      <MantineProvider>
        <MemoryRouter
          initialEntries={[
            {
              pathname: `/products/${product.id}`,
              state: { queueNotice: 'active-attempt-missing' },
            },
          ]}
        >
          <Routes>
            <Route path="/products/:productId" element={<ProductDetailsPage />} />
          </Routes>
        </MemoryRouter>
      </MantineProvider>,
    );

    expect(screen.getByRole('alert')).toHaveTextContent('Эта очередь уже завершилась');
  });
});
