import { MantineProvider } from '@mantine/core';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router';

import type { Product } from '../model/product.schema';
import { ProductCard } from './ProductCard';

const product: Product = {
  allocatable_stock: 3,
  category: 'collectibles',
  description: 'Коллекционный товар',
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

const renderCard = (value: Product = product) =>
  render(
    <MantineProvider>
      <MemoryRouter>
        <ProductCard product={value} />
      </MemoryRouter>
    </MantineProvider>,
  );

describe('ProductCard', () => {
  it('shows user-facing product data without internal stock details', () => {
    renderCard();

    expect(screen.getByRole('heading', { name: product.title })).toBeInTheDocument();
    expect(screen.getByText('14 990 ₽')).toBeInTheDocument();
    expect(screen.getByText('В наличии')).toBeInTheDocument();
    expect(screen.getByText('Свободный остаток: 2')).toBeInTheDocument();
    expect(screen.getByText('В очереди: 2')).toBeInTheDocument();
    expect(screen.queryByText(/reserved/i)).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /купить/i })).not.toBeInTheDocument();
  });

  it.each([
    ['Доступно по очереди', { allocatable_stock: 3, free_stock: 0, queue_enabled: true }],
    ['Нет в наличии', { allocatable_stock: 0, free_stock: 0, queue_enabled: true }],
    ['Покупка временно недоступна', { free_stock: 2, queue_enabled: false }],
  ])('shows "%s" based only on backend stock data', (label, stock) => {
    renderCard({ ...product, ...stock });

    expect(screen.getByText(label)).toBeInTheDocument();
  });

  it('uses the same neutral placeholder when an image is absent', () => {
    renderCard({ ...product, image_url: '' });

    expect(screen.getByRole('img', { name: product.title })).toHaveAttribute(
      'src',
      expect.stringMatching(/^data:image\/svg\+xml/),
    );
  });

  it('navigates to the product route with React Router', async () => {
    const user = userEvent.setup();

    render(
      <MantineProvider>
        <MemoryRouter>
          <Routes>
            <Route path="/" element={<ProductCard product={product} />} />
            <Route path="/products/:productId" element={<div>Страница товара</div>} />
          </Routes>
        </MemoryRouter>
      </MantineProvider>,
    );

    await user.click(screen.getByRole('link', { name: 'Открыть товар' }));

    expect(screen.getByText('Страница товара')).toBeInTheDocument();
  });
});
