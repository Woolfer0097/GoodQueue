import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Link, MemoryRouter, Route, Routes } from 'react-router';

import type { Product } from '@/entities/product';

interface ProductsQueryState {
  data?: Product[];
  error?: unknown;
  isError: boolean;
  isFetching?: boolean;
  isPending: boolean;
  refetch: () => Promise<void>;
}

const useProductsQueryMock = jest.fn<() => ProductsQueryState>();
const refetchMock = jest.fn<() => Promise<void>>();

jest.unstable_mockModule('@/entities/product', () => ({
  ProductCard: ({ product }: { product: Product }) => (
    <Link data-testid="product-card" to={`/products/${product.id}`}>
      {product.title}
    </Link>
  ),
  useProductsQuery: useProductsQueryMock,
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
