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

const useProductQueryMock = jest.fn<(productId: string) => ProductQueryState>();
const refetchMock = jest.fn<() => Promise<void>>();
const joinQueueCtaMock = jest.fn<(productId: string, userId: string | null) => void>();
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
  formatProductPrice: () => '14 990 ₽',
  ProductAvailabilityBadge: () => <span>В наличии</span>,
  PRODUCT_IMAGE_PLACEHOLDER: 'data:image/svg+xml,placeholder',
  useProductQuery: useProductQueryMock,
}));

jest.unstable_mockModule('@/features/join-queue', () => ({
  JoinQueueButton: ({
    onJoined,
    productId,
    userId: currentUserId,
  }: {
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
      Купить
    </button>
  ),
}));

jest.unstable_mockModule('@/features/queue-polling', () => ({
  getQueueAttemptRoute: (currentProductId: string, state: QueueAttempt['state']) =>
    state === 'waiting' ? `/products/${currentProductId}/queue` : '/unexpected',
}));

jest.unstable_mockModule('@/features/select-demo-user', () => ({
  useCurrentDemoUser: () => ({ userId }),
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
    joinQueueCtaMock.mockReset();
    setQueryState();
  });

  it('loads the route product and shows only user-facing details in mobile reading order', () => {
    renderPage();

    expect(useProductQueryMock).toHaveBeenCalledWith(product.id);

    const image = screen.getByRole('img', { name: product.title });
    const price = screen.getByText('14 990 ₽');
    const title = screen.getByRole('heading', { name: product.title });
    const availability = screen.getByText('В наличии');
    const stock = screen.getByText('В наличии: 2');

    expect(screen.getByText('В очереди: 2')).toBeInTheDocument();
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
    expect(screen.queryByText(product.category)).not.toBeInTheDocument();
    const buyButton = screen.getByRole('button', { name: 'Купить' });
    expect(
      stock.compareDocumentPosition(buyButton) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it('passes the loaded product and selected demo user to join-queue', async () => {
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

  it('does not show zero stock or queue counters', () => {
    setQueryState({ data: { ...product, free_stock: 0, waiting_count: 0 } });

    renderPage();

    expect(screen.queryByText(/^В наличии:/)).not.toBeInTheDocument();
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

  it('returns to the catalog with React Router', async () => {
    const user = userEvent.setup();
    renderPage();

    const catalogLink = screen.getByRole('link', { name: 'Вернуться в каталог' });
    expect(catalogLink).toHaveAttribute('href', '/');
    expect(catalogLink.querySelector('svg')).toBeInTheDocument();
    expect(catalogLink).not.toHaveTextContent('Вернуться в каталог');

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

    expect(screen.getByRole('alert')).toHaveTextContent('Активная очередь не найдена');
  });
});
