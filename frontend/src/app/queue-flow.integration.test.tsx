import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { Notifications, notifications } from '@mantine/notifications';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { MemoryRouter, useLocation } from 'react-router';

import type { DemoUser } from '@/entities/demo-user';
import type { Product } from '@/entities/product';
import type { QueueAttempt, QueueAttemptState } from '@/entities/queue-attempt';

import { theme } from './theme/theme';

const API_BASE_URL = 'http://queue-flow.test';
const PRODUCT_ID = '11111111-1111-1111-1111-111111111111';
const ATTEMPT_ID = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';

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
  allocatable_stock: 1,
  category: 'collectibles',
  description: 'Коллекционный товар',
  free_stock: 0,
  id: PRODUCT_ID,
  image_url: 'https://example.com/product.jpg',
  price_cents: 1499000,
  queue_enabled: true,
  reserved: 1,
  title: 'Лимитированный товар',
  waiting_buffer_capacity: 10,
  waiting_count: 2,
};

const alternative: Product = {
  ...product,
  free_stock: 2,
  id: '22222222-2222-2222-2222-222222222222',
  title: 'Доступная альтернатива',
  waiting_count: 0,
};

interface ResponseOptions {
  body?: unknown;
  status?: number;
  statusText?: string;
}

type RequestHandler = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response> | Response;

const handlers = new Map<string, RequestHandler[]>();
const originalFetch = globalThis.fetch;
const fetchMock = jest.fn<typeof fetch>();

Object.assign(globalThis, { fetch: fetchMock });

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

const requestKey = (input: RequestInfo | URL, init?: RequestInit) => {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;

  return `${init?.method ?? 'GET'} ${new URL(url).pathname}`;
};

const addHandlers = (method: string, path: string, ...nextHandlers: RequestHandler[]) => {
  handlers.set(`${method} ${path}`, nextHandlers);
};

const addJsonSequence = (method: string, path: string, ...responses: unknown[]) => {
  addHandlers(
    method,
    path,
    ...responses.map(
      (response) => () =>
        createJsonResponse(
          typeof response === 'object' &&
            response !== null &&
            ('body' in response || 'status' in response || 'statusText' in response)
            ? (response as ResponseOptions)
            : { body: response },
        ),
    ),
  );
};

const makeAttempt = (
  state: QueueAttemptState,
  overrides: Partial<QueueAttempt> = {},
): QueueAttempt => {
  const now = new Date().toISOString();

  return {
    attempt_id: ATTEMPT_ID,
    created_at: now,
    message_code: state,
    next_action: state === 'waiting' ? 'wait' : 'none',
    position: 2,
    position_ahead: 1,
    product_id: PRODUCT_ID,
    queue_sequence: 2,
    state,
    total_waiting: 3,
    updated_at: now,
    ...overrides,
  };
};

jest.unstable_mockModule('@/shared/api/config', () => ({
  API_BASE_URL,
}));

const { CurrentDemoUserProvider } = await import('@/entities/demo-user');
const { AppRouter } = await import('./router/AppRouter');

function LocationProbe() {
  const location = useLocation();

  return <output aria-label="Текущий маршрут">{location.pathname}</output>;
}

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
        <MantineProvider theme={theme}>
          <Notifications limit={3} />
          {children}
        </MantineProvider>
      </CurrentDemoUserProvider>
    </QueryClientProvider>
  );
}

const renderApp = (initialEntry: string) => {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { gcTime: Infinity, retry: false, staleTime: 0 },
    },
  });

  return render(
    <TestProviders queryClient={queryClient}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <LocationProbe />
        <AppRouter />
      </MemoryRouter>
    </TestProviders>,
  );
};

const queueEntryPath = `/api/v1/products/${PRODUCT_ID}/queue-entry`;
const joinPath = `/api/v1/products/${PRODUCT_ID}/queue-entries`;
const checkoutPath = `/api/v1/queue-attempts/${ATTEMPT_ID}/checkout`;

const getCalls = (method: string, path: string) =>
  fetchMock.mock.calls.filter(([input, init]) => requestKey(input, init) === `${method} ${path}`);

const expectCurrentRoute = (path: string) =>
  expect(screen.getByRole('status', { name: 'Текущий маршрут' })).toHaveTextContent(path);

const selectSecondDemoUser = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.click(
    await screen.findByRole('button', {
      name: `Сменить аккаунт: ${users[0].display_name}`,
    }),
  );
  fireEvent.click(screen.getByRole('combobox', { hidden: true }));
  await waitFor(() => expect(screen.getAllByRole('option', { hidden: true })).toHaveLength(2));
  fireEvent.click(screen.getAllByRole('option', { hidden: true })[1]);
  await screen.findByRole('button', {
    name: `Сменить аккаунт: ${users[1].display_name}`,
  });
};

describe('queue flow integration', () => {
  beforeEach(() => {
    handlers.clear();
    localStorage.clear();
    notifications.clean();
    fetchMock.mockReset();
    fetchMock.mockImplementation(async (input, init) => {
      const key = requestKey(input, init);
      const sequence = handlers.get(key);

      if (!sequence?.length) {
        throw new Error(`Unexpected request: ${key}`);
      }

      const handler = sequence.length === 1 ? sequence[0] : sequence.shift()!;

      return handler(input, init);
    });
    addJsonSequence('GET', '/api/v1/demo/users', users);
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}/alternatives`, []);
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  afterAll(() => {
    if (originalFetch === undefined) {
      Reflect.deleteProperty(globalThis, 'fetch');
    } else {
      Object.assign(globalThis, { fetch: originalFetch });
    }
  });

  it('uses the selected demo user and follows waiting -> invited without a reload', async () => {
    jest.useFakeTimers();
    const user = userEvent.setup({
      advanceTimers: (milliseconds) => jest.advanceTimersByTime(milliseconds),
    });
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}`, product);
    addJsonSequence(
      'GET',
      queueEntryPath,
      { body: { error: { code: 'not_found' } }, status: 404, statusText: 'Not Found' },
      { body: { error: { code: 'not_found' } }, status: 404, statusText: 'Not Found' },
      makeAttempt('waiting'),
      makeAttempt('invited', { deadline_at: new Date(Date.now() + 60_000).toISOString() }),
    );
    addJsonSequence('POST', joinPath, makeAttempt('waiting'));

    renderApp(`/products/${PRODUCT_ID}`);
    await selectSecondDemoUser(user);
    await user.click(await screen.findByRole('button', { name: 'Встать в очередь' }));

    expect(await screen.findByRole('heading', { name: 'Вы в очереди' })).toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}/queue`);
    expect(screen.getByText('Место в очереди: 2')).toBeInTheDocument();

    const joinCall = getCalls('POST', joinPath)[0];
    expect(new Headers(joinCall[1]?.headers).get('X-User-ID')).toBe(users[1].external_user_id);
    expect(new Headers(joinCall[1]?.headers).get('Idempotency-Key')).toBeTruthy();

    await act(async () => {
      await jest.advanceTimersByTimeAsync(1_500);
    });

    expect(await screen.findByRole('heading', { name: 'Товар ждёт вас' })).toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}/reservation`);
    expect(screen.getByRole('timer')).toBeInTheDocument();
    const latestPollingCall = getCalls('GET', queueEntryPath).at(-1);
    expect(new Headers(latestPollingCall?.[1]?.headers).get('X-User-ID')).toBe(
      users[1].external_user_id,
    );
  });

  it('follows the fast path to the protected checkout boundary without simulating payment', async () => {
    const user = userEvent.setup();
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}`, product);
    addJsonSequence(
      'GET',
      queueEntryPath,
      {
        body: { error: { code: 'not_found' } },
        status: 404,
        statusText: 'Not Found',
      },
      makeAttempt('checkout'),
    );
    addJsonSequence('POST', joinPath, makeAttempt('checkout'));

    renderApp(`/products/${PRODUCT_ID}`);
    await user.click(await screen.findByRole('button', { name: 'Встать в очередь' }));

    await waitFor(() => expect(getCalls('POST', joinPath)).toHaveLength(1));

    expect(
      await screen.findByRole('heading', { name: 'Товар сохранён за вами' }),
    ).toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}/checkout`);
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Оплата пока недоступна в этой версии сервиса',
    );
    expect(screen.getByRole('button', { name: 'Отказаться от покупки' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Перейти к оплате' })).not.toBeInTheDocument();
    expect(getCalls('POST', joinPath)).toHaveLength(1);
    expect(getCalls('POST', checkoutPath)).toHaveLength(0);
  });

  it('restores an active attempt on the product page without offering a second join', async () => {
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}`, product);
    addJsonSequence('GET', queueEntryPath, makeAttempt('waiting'));

    renderApp(`/products/${PRODUCT_ID}`);

    expect(await screen.findByText('Вы уже в очереди')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Вернуться в очередь' })).toHaveAttribute(
      'href',
      `/products/${PRODUCT_ID}/queue`,
    );
    expect(screen.queryByRole('button', { name: 'Купить' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Встать в очередь' })).not.toBeInTheDocument();
    expect(getCalls('POST', joinPath)).toHaveLength(0);
  });

  it('opens a similar product without starting or cancelling a queue attempt', async () => {
    const user = userEvent.setup();
    const alternativeQueueEntryPath = `/api/v1/products/${alternative.id}/queue-entry`;

    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}`, product);
    addJsonSequence('GET', queueEntryPath, {
      body: { error: { code: 'not_found' } },
      status: 404,
      statusText: 'Not Found',
    });
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}/alternatives`, [alternative]);
    addJsonSequence('GET', `/api/v1/products/${alternative.id}`, alternative);
    addJsonSequence('GET', alternativeQueueEntryPath, {
      body: { error: { code: 'not_found' } },
      status: 404,
      statusText: 'Not Found',
    });
    addJsonSequence('GET', `/api/v1/products/${alternative.id}/alternatives`, []);

    renderApp(`/products/${PRODUCT_ID}`);

    await user.click(
      await screen.findByRole('link', { name: `Открыть товар: ${alternative.title}` }),
    );

    expect(await screen.findByRole('heading', { name: alternative.title })).toBeInTheDocument();
    expectCurrentRoute(`/products/${alternative.id}`);
    expect(getCalls('POST', joinPath)).toHaveLength(0);
    expect(getCalls('DELETE', queueEntryPath)).toHaveLength(0);
  });

  it('reuses the idempotency key after a network failure and hides raw errors', async () => {
    const user = userEvent.setup();
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}`, product);
    addJsonSequence('GET', queueEntryPath, {
      body: { error: { code: 'not_found' } },
      status: 404,
      statusText: 'Not Found',
    });
    addHandlers(
      'POST',
      joinPath,
      () => Promise.reject(new Error('socket exploded with secret backend details')),
      () => createJsonResponse({ body: makeAttempt('waiting') }),
    );

    renderApp(`/products/${PRODUCT_ID}`);
    const buyButton = await screen.findByRole('button', { name: 'Встать в очередь' });
    await user.click(buyButton);

    expect(await screen.findByText('Не удалось войти в очередь')).toBeInTheDocument();
    expect(screen.queryByText(/socket exploded|secret backend details/i)).not.toBeInTheDocument();

    await user.click(buyButton);
    expect(await screen.findByRole('heading', { name: 'Вы в очереди' })).toBeInTheDocument();

    const joinCalls = getCalls('POST', joinPath);
    expect(joinCalls).toHaveLength(2);
    expect(new Headers(joinCalls[0][1]?.headers).get('Idempotency-Key')).toBe(
      new Headers(joinCalls[1][1]?.headers).get('Idempotency-Key'),
    );
  });

  it('keeps queue_disabled as a join error on the product page', async () => {
    const user = userEvent.setup();
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}`, product);
    addJsonSequence('GET', queueEntryPath, {
      body: { error: { code: 'not_found' } },
      status: 404,
      statusText: 'Not Found',
    });
    addJsonSequence('POST', joinPath, {
      body: { error: { code: 'queue_disabled', details: 'raw conflict' } },
      status: 409,
      statusText: 'Conflict',
    });

    renderApp(`/products/${PRODUCT_ID}`);
    await user.click(await screen.findByRole('button', { name: 'Встать в очередь' }));

    expect(await screen.findByText('Покупка временно недоступна')).toBeInTheDocument();
    expect(screen.getByText('Для этого товара очередь сейчас отключена.')).toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}`);
    expect(screen.queryByText(/raw conflict|409|Conflict/)).not.toBeInTheDocument();
  });

  it('explains a concurrent queue_full response without blaming the connection', async () => {
    const user = userEvent.setup();
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}`, product);
    addJsonSequence('GET', queueEntryPath, {
      body: { error: { code: 'not_found' } },
      status: 404,
      statusText: 'Not Found',
    });
    addJsonSequence('POST', joinPath, {
      body: { error: { code: 'queue_full', details: 'raw conflict' } },
      status: 409,
      statusText: 'Conflict',
    });

    renderApp(`/products/${PRODUCT_ID}`);
    await user.click(await screen.findByRole('button', { name: 'Встать в очередь' }));

    expect(await screen.findByText('Очередь заполнена')).toBeInTheDocument();
    expect(screen.getByText('Попробуйте позже или выберите другой товар.')).toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}`);
    expect(
      screen.queryByText(/проверьте соединение|raw conflict|409|Conflict/i),
    ).not.toBeInTheDocument();
  });

  it('restores a waiting attempt from a direct URL and cancels it through the backend', async () => {
    const user = userEvent.setup();
    addJsonSequence('GET', queueEntryPath, makeAttempt('waiting'), makeAttempt('cancelled'));
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}/alternatives`, [alternative]);
    addJsonSequence('DELETE', queueEntryPath, { status: 204 });

    renderApp(`/products/${PRODUCT_ID}/queue`);

    expect(await screen.findByRole('heading', { name: 'Вы в очереди' })).toBeInTheDocument();
    expect(
      await screen.findByRole('link', { name: `Открыть товар: ${alternative.title}` }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Выйти из очереди' }));

    expect(await screen.findByRole('heading', { name: 'Покупка отменена' })).toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}/result`);
    expect(
      screen.getByRole('link', { name: `Открыть товар: ${alternative.title}` }),
    ).toBeInTheDocument();
    expect(getCalls('GET', `/api/v1/products/${PRODUCT_ID}/alternatives`)).toHaveLength(1);
    expect(getCalls('GET', `/api/v1/products/${alternative.id}/alternatives`)).toHaveLength(0);
    const cancelCall = getCalls('DELETE', queueEntryPath)[0];
    expect(new Headers(cancelCall[1]?.headers).get('X-User-ID')).toBe(users[0].external_user_id);
  });

  it('shows sold-out alternatives and stops polling after the terminal response', async () => {
    jest.useFakeTimers();
    addJsonSequence('GET', queueEntryPath, makeAttempt('waiting'), makeAttempt('sold_out'));
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}/alternatives`, [alternative]);

    renderApp(`/products/${PRODUCT_ID}/queue`);
    expect(await screen.findByRole('heading', { name: 'Вы в очереди' })).toBeInTheDocument();

    await act(async () => {
      await jest.advanceTimersByTimeAsync(1_500);
    });

    expect(await screen.findByRole('heading', { name: 'Товар закончился' })).toBeInTheDocument();
    expect(await screen.findByRole('heading', { name: alternative.title })).toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}/result`);
    const requestCountAtTerminal = getCalls('GET', queueEntryPath).length;

    await act(async () => {
      await jest.advanceTimersByTimeAsync(4_500);
    });

    expect(getCalls('GET', queueEntryPath)).toHaveLength(requestCountAtTerminal);
  });

  it('shows the reservation countdown and refetches when it expires', async () => {
    jest.useFakeTimers();
    const deadline = new Date(Date.now() + 1_000).toISOString();
    addJsonSequence(
      'GET',
      queueEntryPath,
      makeAttempt('invited', { deadline_at: deadline }),
      makeAttempt('invite_expired'),
    );

    renderApp(`/products/${PRODUCT_ID}/reservation`);

    expect(await screen.findByRole('heading', { name: 'Товар ждёт вас' })).toBeInTheDocument();
    expect(screen.getByRole('timer')).toHaveAccessibleName('Осталось времени: 00:01');

    await act(async () => {
      await jest.advanceTimersByTimeAsync(1_000);
    });

    expect(
      await screen.findByRole('heading', { name: 'Время резерва истекло' }),
    ).toBeInTheDocument();
    expect(getCalls('GET', queueEntryPath).length).toBeGreaterThanOrEqual(2);
    expectCurrentRoute(`/products/${PRODUCT_ID}/result`);
  });

  it('follows ReservationPage to the protected checkout boundary using the backend attempt', async () => {
    const user = userEvent.setup();
    addJsonSequence('GET', queueEntryPath, makeAttempt('invited'), makeAttempt('checkout'));
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}`, product);
    addJsonSequence(
      'POST',
      checkoutPath,
      makeAttempt('checkout', { deadline_at: new Date(Date.now() + 60_000).toISOString() }),
    );

    renderApp(`/products/${PRODUCT_ID}/reservation`);

    expect(await screen.findByRole('heading', { name: 'Товар ждёт вас' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Продолжить оформление' }));

    expect(
      await screen.findByRole('heading', { name: 'Товар сохранён за вами' }),
    ).toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}/checkout`);
    expect(screen.getByText(/проверьте товар и время резерва/i)).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Оплата пока недоступна в этой версии сервиса',
    );
    expect(screen.getByRole('button', { name: 'Отказаться от покупки' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Перейти к оплате' })).not.toBeInTheDocument();
    expect(getCalls('POST', checkoutPath)).toHaveLength(1);
    for (const checkoutCall of getCalls('POST', checkoutPath)) {
      expect(new Headers(checkoutCall[1]?.headers).get('X-User-ID')).toBe(
        users[0].external_user_id,
      );
    }
  });

  it('routes a purchased payment result received by polling without generating it locally', async () => {
    jest.useFakeTimers();
    addJsonSequence(
      'GET',
      queueEntryPath,
      makeAttempt('checkout', { deadline_at: new Date(Date.now() + 60_000).toISOString() }),
      makeAttempt('purchased'),
    );
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}`, product);

    renderApp(`/products/${PRODUCT_ID}/checkout`);

    expect(
      await screen.findByRole('heading', { name: 'Товар сохранён за вами' }),
    ).toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}/checkout`);

    await act(async () => {
      await jest.advanceTimersByTimeAsync(1_500);
    });

    expect(await screen.findByRole('heading', { name: 'Покупка завершена' })).toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}/result`);
    expect(getCalls('GET', queueEntryPath).length).toBeGreaterThanOrEqual(2);
    expect(getCalls('POST', checkoutPath)).toHaveLength(0);
  });

  it('cancels checkout through the backend and releases the reserved purchase right', async () => {
    const user = userEvent.setup();
    addJsonSequence('GET', queueEntryPath, makeAttempt('checkout'), makeAttempt('cancelled'));
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}`, product);
    addJsonSequence('DELETE', queueEntryPath, { status: 204 });

    renderApp(`/products/${PRODUCT_ID}/checkout`);
    await user.click(await screen.findByRole('button', { name: 'Отказаться от покупки' }));

    expect(await screen.findByRole('heading', { name: 'Покупка отменена' })).toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}/result`);
    expect(getCalls('DELETE', queueEntryPath)).toHaveLength(1);
    expect(getCalls('POST', checkoutPath)).toHaveLength(0);
  });

  it('keeps checkout recoverable after a cancellation error and hides raw details', async () => {
    const user = userEvent.setup();
    addJsonSequence('GET', queueEntryPath, makeAttempt('checkout'));
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}`, product);
    addJsonSequence('DELETE', queueEntryPath, {
      body: { error: { code: 'internal', details: 'reservation secret leaked' } },
      status: 500,
      statusText: 'Internal Server Error',
    });

    renderApp(`/products/${PRODUCT_ID}/checkout`);
    await user.click(await screen.findByRole('button', { name: 'Отказаться от покупки' }));

    expect(await screen.findByText('Не удалось отказаться от покупки')).toBeInTheDocument();
    expect(screen.getByText('Проверьте соединение и попробуйте ещё раз.')).toBeInTheDocument();
    expect(screen.queryByText(/reservation secret|internal|500/i)).not.toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}/checkout`);
    expect(screen.getByRole('button', { name: 'Отказаться от покупки' })).toBeEnabled();
  });

  it('restores CheckoutPage from a direct URL and refetches at its backend deadline', async () => {
    jest.useFakeTimers();
    const deadline = new Date(Date.now() + 1_000).toISOString();
    addJsonSequence(
      'GET',
      queueEntryPath,
      makeAttempt('checkout', { deadline_at: deadline }),
      makeAttempt('checkout_expired'),
    );
    addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}`, product);

    renderApp(`/products/${PRODUCT_ID}/checkout`);

    expect(
      await screen.findByRole('heading', { name: 'Товар сохранён за вами' }),
    ).toBeInTheDocument();
    expect(screen.getByRole('timer')).toHaveAccessibleName('Осталось времени: 00:01');
    expectCurrentRoute(`/products/${PRODUCT_ID}/checkout`);

    await act(async () => {
      await jest.advanceTimersByTimeAsync(1_000);
    });

    expect(
      await screen.findByRole('heading', { name: 'Время оформления истекло' }),
    ).toBeInTheDocument();
    expect(getCalls('GET', queueEntryPath).length).toBeGreaterThanOrEqual(2);
    expect(getCalls('POST', checkoutPath)).toHaveLength(0);
    expectCurrentRoute(`/products/${PRODUCT_ID}/result`);
  });

  it.each([
    ['checkout_expired', 'Время оформления истекло'],
    ['payment_failed', 'Оплата не прошла'],
    ['purchased', 'Покупка завершена'],
  ] as const)('restores %s from backend after a result route remount', async (state, title) => {
    addJsonSequence('GET', queueEntryPath, makeAttempt(state), makeAttempt(state));

    const firstRender = renderApp(`/products/${PRODUCT_ID}/result`);

    expect(await screen.findByRole('heading', { name: title })).toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}/result`);
    firstRender.unmount();

    renderApp(`/products/${PRODUCT_ID}/result`);

    expect(await screen.findByRole('heading', { name: title })).toBeInTheDocument();
    expectCurrentRoute(`/products/${PRODUCT_ID}/result`);
    expect(getCalls('GET', queueEntryPath)).toHaveLength(2);
  });

  it.each(['checkout_expired', 'payment_failed'] as const)(
    'starts a new backend attempt when repeating a purchase after %s',
    async (state) => {
      const user = userEvent.setup();
      addJsonSequence('GET', queueEntryPath, makeAttempt(state), makeAttempt('checkout'));
      addJsonSequence('GET', `/api/v1/products/${PRODUCT_ID}`, product);
      addJsonSequence(
        'POST',
        joinPath,
        makeAttempt('checkout', {
          attempt_id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
          deadline_at: new Date(Date.now() + 60_000).toISOString(),
        }),
      );

      renderApp(`/products/${PRODUCT_ID}/result`);

      await user.click(await screen.findByRole('button', { name: 'Повторить покупку' }));

      expect(
        await screen.findByRole('heading', { name: 'Товар сохранён за вами' }),
      ).toBeInTheDocument();
      expectCurrentRoute(`/products/${PRODUCT_ID}/checkout`);
      expect(getCalls('POST', joinPath)).toHaveLength(1);
      const retryCall = getCalls('POST', joinPath)[0];
      expect(new Headers(retryCall[1]?.headers).get('X-User-ID')).toBe(users[0].external_user_id);
      expect(new Headers(retryCall[1]?.headers).get('Idempotency-Key')).toBeTruthy();
    },
  );

  it('shows a safe server error with retry instead of backend details', async () => {
    addJsonSequence('GET', queueEntryPath, {
      body: { error: { code: 'internal', details: 'database password leaked' } },
      status: 500,
      statusText: 'Internal Server Error',
    });

    renderApp(`/products/${PRODUCT_ID}/queue`);

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Не удалось обновить очередь');
    expect(screen.getByRole('button', { name: 'Повторить' })).toBeInTheDocument();
    expect(screen.queryByText(/database password|internal|500/i)).not.toBeInTheDocument();
  });
});
