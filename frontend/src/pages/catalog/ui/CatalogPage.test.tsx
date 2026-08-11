import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Link, MemoryRouter, Route, Routes } from 'react-router';

import type { Product } from '@/entities/product';
import type { QueueAttempt } from '@/entities/queue-attempt';

interface ProductsQueryState {
  data?: Product[];
  error?: unknown;
  isError: boolean;
  isFetching?: boolean;
  isPending: boolean;
  refetch: () => Promise<void>;
}

const useProductsQueryMock = jest.fn<() => ProductsQueryState>();
const useActiveQueueAttemptsQueryMock = jest.fn<() => { data?: QueueAttempt[] }>();
const useCurrentDemoUserMock = jest.fn<() => { userId: string | null }>();
const refetchMock = jest.fn<() => Promise<void>>();

interface ProductCardStatusMock {
  href: string;
  label: string;
}

jest.unstable_mockModule('@/entities/product', () => ({
  ProductCard: ({
    product,
    userStatus,
  }: {
    product: Product;
    userStatus?: ProductCardStatusMock;
  }) => (
    <Link data-testid="product-card" to={userStatus?.href ?? `/products/${product.id}`}>
      {product.title}
      {userStatus ? <span>{userStatus.label}</span> : null}
    </Link>
  ),
  ProductCardSkeleton: () => <div data-testid="product-card-skeleton" />,
  useProductsQuery: useProductsQueryMock,
}));

jest.unstable_mockModule('@/entities/demo-user', () => ({
  useCurrentDemoUser: useCurrentDemoUserMock,
}));

jest.unstable_mockModule('@/entities/queue-attempt', () => ({
  getQueueAttemptRoute: (productId: string, state: string) =>
    `/products/${productId}/${state === 'waiting' ? 'queue' : state === 'invited' ? 'reservation' : 'checkout'}`,
  useActiveQueueAttemptsQuery: useActiveQueueAttemptsQueryMock,
}));

const { CatalogPage } = await import('./CatalogPage');

const products: Product[] = [
  {
    allocatable_stock: 3,
    category: 'collectibles',
    description: 'Коллекционный товар',
    free_stock: 2,
    id: '11111111-1111-1111-1111-111111111111',
    image_url: 'https://example.com/product-1.jpg',
    price_cents: 1499000,
    queue_enabled: true,
    reserved: 1,
    title: 'Лимитированный товар',
    waiting_buffer_capacity: 3,
    waiting_count: 2,
  },
  {
    allocatable_stock: 1,
    category: 'electronics',
    description: 'Портативная колонка',
    free_stock: 1,
    id: '22222222-2222-2222-2222-222222222222',
    image_url: 'https://example.com/product-2.jpg',
    price_cents: 799000,
    queue_enabled: true,
    reserved: 0,
    title: 'Колонка Mini',
    waiting_buffer_capacity: 1,
    waiting_count: 0,
  },
];

const activeQueueAttempt: QueueAttempt = {
  attempt_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
  created_at: '2026-08-11T08:00:00Z',
  message_code: 'queue_waiting',
  next_action: 'wait',
  position: 2,
  position_ahead: 1,
  product_id: products[0].id,
  queue_sequence: 2,
  state: 'waiting',
  total_waiting: 3,
  updated_at: '2026-08-11T08:00:01Z',
};

const setQueryState = (state: Partial<ProductsQueryState> = {}) => {
  useProductsQueryMock.mockReturnValue({
    data: products,
    isError: false,
    isPending: false,
    refetch: refetchMock,
    ...state,
  });
};

const renderPage = () =>
  render(
    <MantineProvider>
      <MemoryRouter>
        <CatalogPage />
      </MemoryRouter>
    </MantineProvider>,
  );

describe('CatalogPage', () => {
  beforeEach(() => {
    refetchMock.mockReset();
    refetchMock.mockResolvedValue(undefined);
    useProductsQueryMock.mockReset();
    useActiveQueueAttemptsQueryMock.mockReset();
    useActiveQueueAttemptsQueryMock.mockReturnValue({ data: undefined });
    useCurrentDemoUserMock.mockReset();
    useCurrentDemoUserMock.mockReturnValue({ userId: '00000000-0000-4000-8000-000000000001' });
    setQueryState();
  });

  it('loads products through the product entity and renders a card for each product', () => {
    renderPage();

    expect(useProductsQueryMock).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('heading', { name: 'Каталог товаров' })).toBeInTheDocument();
    expect(screen.getAllByTestId('product-card')).toHaveLength(products.length);
    expect(screen.getByRole('link', { name: products[0].title })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: products[1].title })).toBeInTheDocument();
  });

  it('shows Skeleton placeholders during the first load', () => {
    setQueryState({ data: undefined, isPending: true });

    renderPage();

    expect(screen.getByRole('status', { name: 'Загрузка товаров' })).toBeInTheDocument();
    expect(screen.queryByTestId('product-card')).not.toBeInTheDocument();
  });

  it('keeps loaded products visible during a background refetch', () => {
    setQueryState({ isFetching: true });

    renderPage();

    expect(screen.getAllByTestId('product-card')).toHaveLength(products.length);
    expect(screen.queryByRole('status', { name: 'Загрузка товаров' })).not.toBeInTheDocument();
  });

  it('shows the selected user queue status and routes back to the active flow', () => {
    useActiveQueueAttemptsQueryMock.mockReturnValue({ data: [activeQueueAttempt] });

    renderPage();

    expect(screen.getByText('Вы в очереди · место 2')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: new RegExp(products[0].title) })).toHaveAttribute(
      'href',
      `/products/${products[0].id}/queue`,
    );
    expect(screen.getByRole('link', { name: products[1].title })).toHaveAttribute(
      'href',
      `/products/${products[1].id}`,
    );
  });

  it('shows an empty state when the catalog has no products', () => {
    setQueryState({ data: [] });

    renderPage();

    expect(screen.getByText('Товаров пока нет')).toBeInTheDocument();
    expect(screen.getByText('Загляните позже — каталог скоро обновится.')).toBeInTheDocument();
  });

  it('shows a safe error state and retries the request', async () => {
    const user = userEvent.setup();
    setQueryState({
      data: undefined,
      error: new Error('HTTP request failed: 500 Internal Server Error'),
      isError: true,
    });

    renderPage();

    expect(screen.getByRole('alert')).toHaveTextContent('Не удалось загрузить товары');
    expect(screen.queryByText(/HTTP request failed/i)).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Повторить' }));

    expect(refetchMock).toHaveBeenCalledTimes(1);
  });

  it('navigates to the selected product with React Router', async () => {
    const user = userEvent.setup();

    render(
      <MantineProvider>
        <MemoryRouter>
          <Routes>
            <Route path="/" element={<CatalogPage />} />
            <Route path="/products/:productId" element={<div>Страница товара</div>} />
          </Routes>
        </MemoryRouter>
      </MantineProvider>,
    );

    const productLink = screen.getByRole('link', { name: products[0].title });
    expect(productLink).toHaveAttribute('href', `/products/${products[0].id}`);

    await user.click(productLink);

    expect(screen.getByText('Страница товара')).toBeInTheDocument();
  });
});
