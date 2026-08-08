import { jest } from '@jest/globals';
import { MantineProvider } from '@mantine/core';
import { act, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';

import type { QueueAttempt, QueueAttemptState } from '@/entities/queue-attempt';

interface QueueAttemptQueryState {
  data?: QueueAttempt | null;
  error?: unknown;
  isError: boolean;
  isFetching: boolean;
  isPending: boolean;
  refetch: () => Promise<void>;
}

const productId = '11111111-1111-4111-8111-111111111111';
const userId = '00000000-0000-4000-8000-000000000002';
const useQueueAttemptQueryMock =
  jest.fn<(currentProductId: string, currentUserId: string | null) => QueueAttemptQueryState>();
const refetchMock = jest.fn<() => Promise<void>>();
const relevantProductsMock = jest.fn<(currentProductId: string) => void>();

jest.unstable_mockModule('@/entities/demo-user', () => ({
  useCurrentDemoUser: () => ({ userId }),
}));

jest.unstable_mockModule('@/entities/queue-attempt', () => ({
  getQueueAttemptRoute: (currentProductId: string, state: QueueAttemptState) =>
    state === 'invited'
      ? `/products/${currentProductId}/reservation`
      : state === 'checkout'
        ? `/products/${currentProductId}/checkout`
        : state === 'waiting'
          ? `/products/${currentProductId}/queue`
          : `/products/${currentProductId}/result`,
  useQueueAttemptQuery: useQueueAttemptQueryMock,
}));

jest.unstable_mockModule('@/features/cancel-queue', () => ({
  CancelQueueButton: ({
    productId: currentProductId,
    userId: currentUserId,
  }: {
    productId: string;
    userId: string | null;
  }) => (
    <button type="button">
      Выйти из очереди {currentProductId} {currentUserId}
    </button>
  ),
}));

jest.unstable_mockModule('@/widgets/relevant-products', () => ({
  RelevantProducts: ({ productId: currentProductId }: { productId: string }) => {
    relevantProductsMock(currentProductId);

    return <h2>Похожие товары</h2>;
  },
}));

const { QueuePage } = await import('./QueuePage');

const createAttempt = (state: QueueAttemptState = 'waiting'): QueueAttempt => ({
  attempt_id: '22222222-2222-4222-8222-222222222222',
  created_at: '2026-08-07T10:00:00Z',
  message_code: 'queue_waiting',
  next_action: 'wait',
  position: 2,
  position_ahead: 1,
  product_id: productId,
  queue_sequence: 2,
  state,
  total_waiting: 5,
  updated_at: '2026-08-07T10:00:01Z',
});

const setQueryState = (state: Partial<QueueAttemptQueryState> = {}) => {
  useQueueAttemptQueryMock.mockReturnValue({
    data: createAttempt(),
    isError: false,
    isFetching: false,
    isPending: false,
    refetch: refetchMock,
    ...state,
  });
};

function ProductRoute() {
  const location = useLocation();
  const notice = (location.state as { queueNotice?: string } | null)?.queueNotice;

  return <div>Страница товара {notice}</div>;
}

const renderPage = () =>
  render(
    <MantineProvider>
      <MemoryRouter initialEntries={[`/products/${productId}/queue`]}>
        <Routes>
          <Route path="/products/:productId" element={<ProductRoute />} />
          <Route path="/products/:productId/queue" element={<QueuePage />} />
          <Route path="/products/:productId/reservation" element={<div>Резерв</div>} />
          <Route path="/products/:productId/checkout" element={<div>Оформление</div>} />
          <Route path="/products/:productId/result" element={<div>Результат</div>} />
        </Routes>
      </MemoryRouter>
    </MantineProvider>,
  );

describe('QueuePage', () => {
  beforeEach(() => {
    refetchMock.mockReset();
    refetchMock.mockResolvedValue(undefined);
    relevantProductsMock.mockReset();
    useQueueAttemptQueryMock.mockReset();
    setQueryState();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('loads the attempt from the route and explains automatic waiting updates', () => {
    renderPage();

    expect(useQueueAttemptQueryMock).toHaveBeenCalledWith(productId, userId);
    expect(screen.getByRole('heading', { name: 'Вы в очереди' })).toBeInTheDocument();
    expect(
      screen.getByText(
        'Оставьте страницу открытой. Когда подойдёт ваша очередь, мы проверим наличие товара. Если товар останется, вы сможете продолжить оформление.',
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /выйти из очереди/i })).toBeInTheDocument();
    expect(screen.queryByText(/^waiting$/i)).not.toBeInTheDocument();
  });

  it('shows similar products for the source product after the primary waiting action', () => {
    renderPage();

    const cancelButton = screen.getByRole('button', { name: /выйти из очереди/i });
    const relevantProductsHeading = screen.getByRole('heading', { name: 'Похожие товары' });

    expect(relevantProductsMock).toHaveBeenCalledWith(productId);
    expect(
      cancelButton.compareDocumentPosition(relevantProductsHeading) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it('shows position and total waiting when backend provides them', () => {
    renderPage();

    expect(screen.getByText('Место в очереди: 2')).toBeInTheDocument();
    expect(screen.getByText('Всего ожидают: 5')).toBeInTheDocument();
    expect(screen.queryByText(/примерное время|eta/i)).not.toBeInTheDocument();
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
  });

  it('updates the time spent in the queue every second', () => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date('2026-08-07T10:00:01Z'));

    renderPage();

    expect(screen.getByRole('timer', { name: 'Вы ждёте: 00:01' })).toBeInTheDocument();

    act(() => {
      jest.advanceTimersByTime(1_000);
    });

    expect(screen.getByRole('timer', { name: 'Вы ждёте: 00:02' })).toBeInTheDocument();
  });

  it('omits queue counters that backend did not provide', () => {
    setQueryState({ data: { ...createAttempt(), position: undefined, total_waiting: undefined } });

    renderPage();

    expect(screen.queryByText(/^Место в очереди:/)).not.toBeInTheDocument();
    expect(screen.queryByText(/^Всего ожидают:/)).not.toBeInTheDocument();
  });

  it('shows a skeleton only during the first load', () => {
    setQueryState({ data: undefined, isFetching: true, isPending: true });

    renderPage();

    expect(screen.getByRole('status', { name: 'Загрузка очереди' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Вы в очереди' })).not.toBeInTheDocument();
  });

  it('keeps waiting content visible during background polling', () => {
    setQueryState({ isFetching: true });

    renderPage();

    expect(screen.getByRole('heading', { name: 'Вы в очереди' })).toBeInTheDocument();
    expect(screen.queryByRole('status', { name: 'Загрузка очереди' })).not.toBeInTheDocument();
  });

  it('returns a direct visit without an active attempt to the product with a notice', async () => {
    setQueryState({ data: null });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText('Страница товара active-attempt-missing')).toBeInTheDocument();
    });
  });

  it.each([
    ['invited', 'Резерв'],
    ['checkout', 'Оформление'],
    ['cancelled', 'Результат'],
  ] as const)('redirects backend state %s to its actual screen', async (state, screenName) => {
    setQueryState({ data: createAttempt(state) });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText(screenName)).toBeInTheDocument();
    });
  });
});
