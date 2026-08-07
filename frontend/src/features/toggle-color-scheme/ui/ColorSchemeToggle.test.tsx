import { MantineProvider } from '@mantine/core';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { ColorSchemeToggle } from './ColorSchemeToggle';

describe('ColorSchemeToggle', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('switches between light and dark color schemes', async () => {
    const user = userEvent.setup();

    render(
      <MantineProvider>
        <ColorSchemeToggle />
      </MantineProvider>,
    );

    const toggle = screen.getByRole('button', { name: 'Включить тёмную тему' });

    await user.click(toggle);

    expect(document.documentElement).toHaveAttribute('data-mantine-color-scheme', 'dark');
    expect(screen.getByRole('button', { name: 'Включить светлую тему' })).toBeInTheDocument();
  });
});
