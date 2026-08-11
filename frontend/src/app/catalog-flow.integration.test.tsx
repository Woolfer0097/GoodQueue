import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { act, type ReactNode } from 'react';
import { MemoryRouter } from 'react-router';

import type { DemoUser } from '@/entities/demo-user';
import type { Product } from '@/entities/product';

import { theme } from './theme/theme';

const API_BASE_URL = 'http://localhost:2001';

const users: DemoUser[] = [
  {
    display_name: 'Алексей',
    external_user_id: '00000000-0000-4000-8000-000000000001',
  },
  {
    display_name: 'Мария',
    external_user_id: '00000000-0000-4000-8000-000000000002',
  },
];

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

interface ResponseOptions {
  body?: unknown;
  status?: number;
  statusText?: string;
}

type RequestHandler = () => Promise<Response> | Response;

const handlers = new Map<string, RequestHandler>();
const originalFetch = globalThis.fetch;
const fetchMock = jest.fn<typeof fetch>();
const originalConsoleWarn = console.warn;
const consoleWarnSpy = jest.spyOn(console, 'warn');

Object.assign(globalThis, { fetch: fetchMock });
consoleWarnSpy.mockImplementation((message: unknown, ...optionalParams: unknown[]) => {
  if (
    typeof message === 'string' &&
    message.startsWith('[@mantine/hooks/use-focus-trap] Failed to find focusable element')
  ) {
    return;
  }

  originalConsoleWarn(message, ...optionalParams);
});

const createJsonResponse = ({
  body,
  status = 200,
  statusText = '',
}: ResponseOptions = {}): Response =>
  ({
    headers: {
      get: (name: string) => (name.toLowerCase() === 'content-type' ? 'application/json' : null),
    },
    ok: status >= 200 && status < 300,
    status,
    statusText,
    text: () => Promise.resolve(body === undefined ? '' : JSON.stringify(body)),
  }) as Response;

const getRequestPath = (input: RequestInfo | URL) => {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;

  return new URL(url).pathname;
};

const addJsonHandler = (path: string, body: unknown, status = 200) => {
  handlers.set(path, () => createJsonResponse({ body, status }));
};

const createDeferred = <Value,>() => {
  let resolve!: (value: Value) => void;
  const promise = new Promise<Value>((resolvePromise) => {
    resolve = resolvePromise;
  });

  return { promise, resolve };
};

jest.unstable_mockModule('@/shared/api/config', () => ({
  API_BASE_URL,
}));

const { CurrentDemoUserProvider } = await import('@/entities/demo-user');
const { AppRouter } = await import('./router/AppRouter');

function TestProviders({
  children,
  queryClient,
}: {
  children: ReactNode;
  queryClient: QueryClient;
}) {
  return (
    <QueryClientProvider client={queryClient}>
      <CurrentDemoUserProvider>
        <MantineProvider theme={theme}>{children}</MantineProvider>
      </CurrentDemoUserProvider>
    </QueryClientProvider>
  );
}

const renderApp = (initialEntry = '/') => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return render(
    <TestProviders queryClient={queryClient}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <AppRouter />
      </MemoryRouter>
    </TestProviders>,
  );
};

const selectDemoUser = async (
  user: ReturnType<typeof userEvent.setup>,
  currentUser: DemoUser,
  targetUserIndex: number,
) => {
  await user.click(
    screen.getByRole('button', {
      name: `Сменить аккаунт: ${currentUser.display_name}`,
    }),
  );
  fireEvent.click(screen.getByRole('combobox', { hidden: true }));
  await waitFor(() => {
    expect(screen.getAllByRole('option', { hidden: true })).toHaveLength(users.length);
  });
  fireEvent.click(screen.getAllByRole('option', { hidden: true })[targetUserIndex]);
};

describe('catalog user flow', () => {
  beforeEach(() => {
    handlers.clear();
    localStorage.clear();
    fetchMock.mockReset();
    fetchMock.mockImplementation(async (input) => {
      const path = getRequestPath(input);
      const handler = handlers.get(path);

      if (!handler) {
        throw new Error(`Unexpected request: ${path}`);
      }

      return handler();
    });
    addJsonHandler('/api/v1/demo/users', users);
  });

  afterAll(() => {
    if (originalFetch === undefined) {
      Reflect.deleteProperty(globalThis, 'fetch');
    } else {
      Object.assign(globalThis, { fetch: originalFetch });
    }
    consoleWarnSpy.mockRestore();
  });

  it('loads users and catalog, persists a selected user and opens a product', async () => {
    const user = userEvent.setup();
    addJsonHandler('/api/v1/products', [product]);
    addJsonHandler(`/api/v1/products/${product.id}`, product);

    renderApp();

    expect(
      await screen.findByRole('button', {
        name: `Сменить аккаунт: ${users[0].display_name}`,
      }),
    ).toBeInTheDocument();
    expect(await screen.findByRole('heading', { name: 'Каталог товаров' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: product.title })).toBeInTheDocument();
    expect(screen.getByText('14 990 ₽')).toBeInTheDocument();
    expect(screen.getByText('В очереди: 2')).toBeInTheDocument();
    expect(screen.getByText('В наличии')).toBeInTheDocument();

    await selectDemoUser(user, users[0], 1);

    expect(
      await screen.findByRole('button', {
        name: `Сменить аккаунт: ${users[1].display_name}`,
      }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole('link', { name: `Открыть товар: ${product.title}` }));

    expect(await screen.findByRole('heading', { name: product.title })).toBeInTheDocument();
    expect(screen.getByText('14 990 ₽')).toBeInTheDocument();
    expect(screen.getByText('Осталось: 2')).toBeInTheDocument();
    expect(screen.getByText('В наличии')).toBeInTheDocument();
    expect(screen.queryByText(/^В очереди:/)).not.toBeInTheDocument();
    expect(screen.queryByText(/распределени|лимит очереди/i)).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(
      `${API_BASE_URL}/api/v1/products/${product.id}`,
      undefined,
    );
  });

  it('restores the selected user and opens a product URL directly', async () => {
    const user = userEvent.setup();
    addJsonHandler('/api/v1/products', [product]);
    addJsonHandler(`/api/v1/products/${product.id}`, product);

    const catalog = renderApp();

    await screen.findByRole('button', {
      name: `Сменить аккаунт: ${users[0].display_name}`,
    });
    await selectDemoUser(user, users[0], 1);
    expect(
      await screen.findByRole('button', {
        name: `Сменить аккаунт: ${users[1].display_name}`,
      }),
    ).toBeInTheDocument();

    catalog.unmount();
    fetchMock.mockClear();
    renderApp(`/products/${product.id}`);

    expect(
      await screen.findByRole('button', {
        name: `Сменить аккаунт: ${users[1].display_name}`,
      }),
    ).toBeInTheDocument();
    expect(await screen.findByRole('heading', { name: product.title })).toBeInTheDocument();
    expect(screen.getByText('Осталось: 2')).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalledWith(`${API_BASE_URL}/api/v1/products`, undefined);
  });

  it('shows catalog loading and then an empty state', async () => {
    const response = createDeferred<Response>();
    handlers.set('/api/v1/products', () => response.promise);

    renderApp();

    expect(await screen.findByRole('status', { name: 'Загрузка товаров' })).toBeInTheDocument();

    await act(async () => {
      response.resolve(createJsonResponse({ body: [] }));
      await response.promise;
    });

    expect(await screen.findByText('Товаров пока нет')).toBeInTheDocument();
  });

  it('shows a safe catalog error without exposing the API error', async () => {
    handlers.set('/api/v1/products', () =>
      createJsonResponse({
        body: { error: 'internal details' },
        status: 500,
        statusText: 'Internal Server Error',
      }),
    );

    renderApp();

    expect(await screen.findByRole('alert')).toHaveTextContent('Не удалось загрузить товары');
    expect(
      screen.queryByText(/internal details|500|Internal Server Error/i),
    ).not.toBeInTheDocument();
  });

  it('shows the dedicated state for an unknown product', async () => {
    const unknownProductId = '99999999-9999-4999-8999-999999999999';
    handlers.set(`/api/v1/products/${unknownProductId}`, () =>
      createJsonResponse({
        body: { error: { code: 'not_found' } },
        status: 404,
        statusText: 'Not Found',
      }),
    );

    renderApp(`/products/${unknownProductId}`);

    expect(await screen.findByRole('heading', { name: 'Товар не найден' })).toBeInTheDocument();
    expect(screen.queryByText('Не удалось загрузить товар')).not.toBeInTheDocument();
  });

  it('shows the dedicated not-found state for an invalid product URL', async () => {
    const invalidProductId = '11111111-1111-1111-1111-1111111111112';
    handlers.set(`/api/v1/products/${invalidProductId}`, () =>
      createJsonResponse({
        body: { error: { code: 'invalid_input' } },
        status: 400,
        statusText: 'Bad Request',
      }),
    );

    renderApp(`/products/${invalidProductId}`);

    expect(await screen.findByRole('heading', { name: 'Товар не найден' })).toBeInTheDocument();
    expect(screen.getByText('404')).toBeInTheDocument();
    expect(
      screen.getByText('Возможно, товар удалили или в ссылке есть опечатка.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Вернуться в каталог' })).toHaveAttribute('href', '/');
    expect(screen.queryByRole('navigation', { name: 'Хлебные крошки' })).not.toBeInTheDocument();
    expect(screen.queryByText('Не удалось загрузить товар')).not.toBeInTheDocument();
  });

  it('returns from an unknown frontend route to the catalog', async () => {
    const user = userEvent.setup();
    addJsonHandler('/api/v1/products', [product]);

    renderApp('/unknown-route');

    expect(
      await screen.findByRole('heading', { name: 'Такой страницы не существует' }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole('link', { name: 'Вернуться в каталог' }));

    expect(await screen.findByRole('heading', { name: 'Каталог товаров' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: product.title })).toBeInTheDocument();
  });
});
