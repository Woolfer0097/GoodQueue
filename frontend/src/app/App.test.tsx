import { MantineProvider } from '@mantine/core';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

import { App } from './App';
import { theme } from './theme/theme';

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
  });
});
