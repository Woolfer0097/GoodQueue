import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Link, MemoryRouter } from 'react-router';

import { theme } from '../theme/theme';

jest.unstable_mockModule('@/features/select-demo-user', () => ({
  DemoUserSelect: () => null,
}));

const queuePollingRouteMock = jest.fn();

jest.unstable_mockModule('@/features/queue-polling', async () => {
  const { Outlet } = await import('react-router');

  return {
    QueuePollingRoute: () => {
      queuePollingRouteMock();

      return <Outlet />;
    },
  };
});

jest.unstable_mockModule('@/pages/catalog', () => ({
  CatalogPage: () => (
    <>
      <div>CatalogPage</div>
      <Link to="/products/product-42">Открыть товар</Link>
    </>
  ),
}));

jest.unstable_mockModule('@/pages/product-details', () => ({
  ProductDetailsPage: () => <div>ProductDetailsPage</div>,
}));

jest.unstable_mockModule('@/pages/queue', () => ({
  QueuePage: () => <div>QueuePage</div>,
}));

jest.unstable_mockModule('@/pages/reservation', () => ({
  ReservationPage: () => <div>ReservationPage</div>,
}));

jest.unstable_mockModule('@/pages/checkout', () => ({
  CheckoutPage: () => <div>CheckoutPage</div>,
}));

jest.unstable_mockModule('@/pages/result', () => ({
  ResultPage: () => <div>ResultPage</div>,
}));

jest.unstable_mockModule('@/pages/not-found', () => ({
  NotFoundPage: () => <div>NotFoundPage</div>,
}));

const { AppRouter } = await import('./AppRouter');

const renderRoute = (path: string) =>
  render(
    <MantineProvider theme={theme}>
      <MemoryRouter initialEntries={[path]}>
        <AppRouter />
      </MemoryRouter>
    </MantineProvider>,
  );

describe('AppRouter', () => {
  beforeEach(() => {
    queuePollingRouteMock.mockClear();
  });

  it('renders CatalogPage at the root route', () => {
    renderRoute('/');

    expect(screen.getByText('CatalogPage')).toBeInTheDocument();
  });

  it('renders ProductDetailsPage when a product URL is opened directly', () => {
    renderRoute('/products/product-42');

    expect(screen.getByText('ProductDetailsPage')).toBeInTheDocument();
  });

  it('navigates from the catalog to ProductDetailsPage with React Router', async () => {
    const user = userEvent.setup();
    renderRoute('/');

    await user.click(screen.getByRole('link', { name: 'Открыть товар' }));

    expect(screen.getByText('ProductDetailsPage')).toBeInTheDocument();
  });

  it('renders NotFoundPage for an unknown frontend URL', () => {
    renderRoute('/unknown-page');

    expect(screen.getByText('NotFoundPage')).toBeInTheDocument();
  });

  it.each([
    ['/products/product-42/queue', 'QueuePage'],
    ['/products/product-42/reservation', 'ReservationPage'],
    ['/products/product-42/checkout', 'CheckoutPage'],
    ['/products/product-42/result', 'ResultPage'],
  ])('keeps direct route %s behind the backend attempt check', (path, pageName) => {
    renderRoute(path);

    expect(screen.getByText(pageName)).toBeInTheDocument();
    expect(queuePollingRouteMock).toHaveBeenCalled();
  });
});
