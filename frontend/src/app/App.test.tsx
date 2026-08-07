import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

import { theme } from './theme/theme';

const DemoUserSelectMock = jest.fn(() => <div>Demo user selector</div>);
const ColorSchemeToggleMock = jest.fn(() => <button type="button">Theme toggle</button>);

jest.unstable_mockModule('@/features/select-demo-user', () => ({
  DemoUserSelect: DemoUserSelectMock,
}));

jest.unstable_mockModule('@/features/toggle-color-scheme', () => ({
  ColorSchemeToggle: ColorSchemeToggleMock,
}));

jest.unstable_mockModule('@/pages/catalog', () => ({
  CatalogPage: () => <div>Product catalog</div>,
}));

jest.unstable_mockModule('@/pages/product-details', () => ({
  ProductDetailsPage: () => <div>Product details</div>,
}));

const { App } = await import('./App');

describe('App', () => {
  it('renders the application shell', () => {
    render(
      <MantineProvider theme={theme}>
        <MemoryRouter>
          <App />
        </MemoryRouter>
      </MantineProvider>,
    );

    expect(screen.getByRole('main')).toBeInTheDocument();
    expect(screen.queryByRole('banner')).not.toBeInTheDocument();
    expect(screen.getByText('Demo user selector')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Theme toggle' })).toBeInTheDocument();

    const controls = screen.getByRole('button', { name: 'Theme toggle' }).parentElement;
    expect(controls?.firstElementChild).toHaveTextContent('Theme toggle');
    expect(controls?.lastElementChild).toHaveTextContent('Demo user selector');
  });
});
