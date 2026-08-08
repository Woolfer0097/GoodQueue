import { MantineProvider } from '@mantine/core';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router';

import { NotFoundPage } from './NotFoundPage';

const renderPage = () =>
  render(
    <MantineProvider>
      <MemoryRouter initialEntries={['/unknown-page']}>
        <Routes>
          <Route path="/" element={<div>Каталог товаров</div>} />
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </MemoryRouter>
    </MantineProvider>,
  );

describe('NotFoundPage', () => {
  it('shows a concise 404 state and returns to the catalog with React Router', async () => {
    const user = userEvent.setup();
    renderPage();

    expect(screen.getByText('404')).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { name: 'Такой страницы не существует' }),
    ).toBeInTheDocument();
    expect(
      screen.getByText('Возможно, страницу удалили или в ссылке есть опечатка.'),
    ).toBeInTheDocument();

    const catalogLink = screen.getByRole('link', { name: 'Вернуться в каталог' });
    expect(catalogLink).toHaveAttribute('href', '/');

    await user.click(catalogLink);

    expect(screen.getByText('Каталог товаров')).toBeInTheDocument();
  });
});
