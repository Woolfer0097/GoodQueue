import { MantineProvider } from '@mantine/core';
import { fireEvent, render, screen } from '@testing-library/react';
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
    expect(screen.getByText('В наличии: 2')).toBeInTheDocument();
    expect(screen.getByText('В очереди: 2')).toBeInTheDocument();
    expect(screen.queryByText(/reserved/i)).not.toBeInTheDocument();
    expect(screen.queryByText(product.id)).not.toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
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
    expect(screen.getByTestId('product-image-skeleton')).not.toHaveAttribute('data-visible');
  });

  it('shows a Skeleton until the product image is loaded', () => {
    renderCard();

    const image = screen.getByRole('img', { name: product.title });
    const skeleton = screen.getByTestId('product-image-skeleton');

    expect(skeleton).toHaveAttribute('data-visible');

    fireEvent.load(image);

    expect(skeleton).not.toHaveAttribute('data-visible');
  });

  it('does not show an empty queue counter', () => {
    renderCard({ ...product, waiting_count: 0 });

    expect(screen.queryByText(/^В очереди:/)).not.toBeInTheDocument();
  });

  it('supports hover styling and keyboard focus without inline styles', async () => {
    const user = userEvent.setup();
    renderCard();

    const productLink = screen.getByRole('link', {
      name: `Открыть товар: ${product.title}`,
    });
    const title = screen.getByRole('heading', { name: product.title });

    expect(productLink).toHaveClass('card');
    expect(title).toHaveClass('title');

    await user.hover(productLink);
    await user.unhover(productLink);
    await user.tab();

    expect(productLink).toHaveFocus();
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

    const productLink = screen.getByRole('link', {
      name: `Открыть товар: ${product.title}`,
    });

    expect(productLink).toHaveAttribute('href', `/products/${product.id}`);

    await user.click(productLink);

    expect(screen.getByText('Страница товара')).toBeInTheDocument();
  });
});
