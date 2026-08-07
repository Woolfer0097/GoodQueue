import { MantineProvider } from '@mantine/core';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router';

import { ProductBreadcrumbs } from './ProductBreadcrumbs';

const productId = '11111111-1111-1111-1111-111111111111';

const renderBreadcrumbs = (currentPage?: string, productTitle?: string) =>
  render(
    <MantineProvider>
      <MemoryRouter initialEntries={[`/products/${productId}/queue`]}>
        <ProductBreadcrumbs
          currentPage={currentPage}
          productId={productId}
          productTitle={productTitle}
        />
        <Routes>
          <Route path="/" element={<div>Страница каталога</div>} />
          <Route path="/products/:productId" element={<div>Страница товара</div>} />
        </Routes>
      </MemoryRouter>
    </MantineProvider>,
  );

describe('ProductBreadcrumbs', () => {
  it('shows catalog and the loaded product title on the product page', () => {
    renderBreadcrumbs(undefined, 'Лимитированный товар');

    expect(screen.getByRole('navigation', { name: 'Хлебные крошки' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Каталог' })).toHaveAttribute('href', '/');
    expect(screen.getByText('Лимитированный товар')).toHaveAttribute('aria-current', 'page');
    expect(screen.queryByRole('link', { name: 'Лимитированный товар' })).not.toBeInTheDocument();
    expect(screen.getByRole('navigation', { name: 'Хлебные крошки' })).not.toHaveTextContent('/');
    expect(
      screen.getByRole('navigation', { name: 'Хлебные крошки' }).querySelector('svg'),
    ).toHaveAttribute('aria-hidden', 'true');
  });

  it('links the product and marks the purchase-flow step as current', () => {
    renderBreadcrumbs('Очередь');

    expect(screen.getByRole('link', { name: 'Товар' })).toHaveAttribute(
      'href',
      `/products/${productId}`,
    );
    expect(screen.getByText('Очередь')).toHaveAttribute('aria-current', 'page');
    expect(screen.queryByRole('link', { name: 'Очередь' })).not.toBeInTheDocument();
  });

  it('navigates with React Router links', async () => {
    const user = userEvent.setup();
    renderBreadcrumbs('Оформление', 'Лимитированный товар');

    await user.click(screen.getByRole('link', { name: 'Лимитированный товар' }));
    expect(screen.getByText('Страница товара')).toBeInTheDocument();

    await user.click(screen.getByRole('link', { name: 'Каталог' }));
    expect(screen.getByText('Страница каталога')).toBeInTheDocument();
  });
});
