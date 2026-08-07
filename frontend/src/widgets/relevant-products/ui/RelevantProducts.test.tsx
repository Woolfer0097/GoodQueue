import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import type { Product } from '@/entities/product';

interface AlternativesQueryState {
  data?: Product[];
  isError: boolean;
  isPending: boolean;
  refetch: () => Promise<void>;
}

const useProductAlternativesQueryMock = jest.fn<(productId: string) => AlternativesQueryState>();
const refetchMock = jest.fn<() => Promise<void>>();

jest.unstable_mockModule('@/entities/product', () => ({
  ProductCard: ({ product }: { product: Product }) => <div>{product.title}</div>,
  ProductCardSkeleton: () => <div data-testid="product-card-skeleton" />,
  useProductAlternativesQuery: useProductAlternativesQueryMock,
}));

const { RelevantProducts } = await import('./RelevantProducts');

const currentProductId = '11111111-1111-1111-1111-111111111111';
const alternative: Product = {
  allocatable_stock: 3,
  category: 'collectibles',
  description: 'Доступный похожий товар',
  free_stock: 3,
  id: '22222222-2222-2222-2222-222222222222',
  image_url: 'https://example.com/alternative.jpg',
  price_cents: 1299000,
  queue_enabled: true,
  reserved: 0,
  title: 'Похожий товар',
  waiting_buffer_capacity: 3,
  waiting_count: 0,
};

const currentProduct: Product = {
  ...alternative,
  id: currentProductId,
  title: 'Исходный товар',
};

const setQueryState = (state: Partial<AlternativesQueryState> = {}) => {
  useProductAlternativesQueryMock.mockReturnValue({
    data: [alternative],
    isError: false,
    isPending: false,
    refetch: refetchMock,
    ...state,
  });
};

const renderWidget = () =>
  render(
    <MantineProvider>
      <RelevantProducts productId={currentProductId} />
    </MantineProvider>,
  );

describe('RelevantProducts', () => {
  beforeEach(() => {
    refetchMock.mockReset();
    refetchMock.mockResolvedValue(undefined);
    useProductAlternativesQueryMock.mockReset();
    setQueryState();
  });

  it('shows backend alternatives with the user-facing title and excludes the source product', () => {
    setQueryState({ data: [currentProduct, alternative] });

    renderWidget();

    expect(useProductAlternativesQueryMock).toHaveBeenCalledWith(currentProductId);
    expect(screen.getByRole('heading', { name: 'Похожие товары' })).toBeInTheDocument();
    expect(screen.getByText(alternative.title)).toBeInTheDocument();
    expect(screen.queryByText(currentProduct.title)).not.toBeInTheDocument();
  });

  it('shows product-shaped skeletons only during the initial load', () => {
    setQueryState({ data: undefined, isPending: true });

    renderWidget();

    expect(screen.getByRole('status', { name: 'Загрузка похожих товаров' })).toBeInTheDocument();
    expect(screen.getAllByTestId('product-card-skeleton')).toHaveLength(4);
  });

  it('shows a quiet empty state', () => {
    setQueryState({ data: [] });

    renderWidget();

    expect(screen.getByText('Похожих товаров пока нет.')).toBeInTheDocument();
  });

  it('keeps the secondary failure recoverable without raw error details', async () => {
    const user = userEvent.setup();
    setQueryState({ data: undefined, isError: true });

    renderWidget();

    expect(screen.getByRole('alert')).toHaveTextContent('Не удалось загрузить похожие товары');
    await user.click(screen.getByRole('button', { name: 'Повторить' }));

    expect(refetchMock).toHaveBeenCalledTimes(1);
  });
});
