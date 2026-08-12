import { jest } from '@jest/globals';
import { render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { MemoryRouter, useLocation } from 'react-router';

import type { QueueAttempt, QueueAttemptState } from '@/entities/queue-attempt';

let activeAttempts: QueueAttempt[] | undefined;
let userId: string | null = '00000000-0000-4000-8000-000000000001';

const getQueueAttemptMock =
  jest.fn<(productId: string, currentUserId: string) => Promise<QueueAttempt | null>>();
const showNotificationMock = jest.fn<(notification: Record<string, unknown>) => void>();
const updateNotificationMock = jest.fn<(notification: Record<string, unknown>) => void>();
const hideNotificationMock = jest.fn<(id: string) => void>();

jest.unstable_mockModule('@mantine/notifications', () => ({
  notifications: {
    hide: hideNotificationMock,
    show: showNotificationMock,
    update: updateNotificationMock,
  },
}));

jest.unstable_mockModule('@/entities/demo-user', () => ({
  useCurrentDemoUser: () => ({ userId }),
}));

jest.unstable_mockModule('@/entities/queue-attempt', () => ({
  getQueueAttempt: getQueueAttemptMock,
  getQueueAttemptRoute: (productId: string, state: QueueAttemptState) =>
    state === 'waiting'
      ? `/products/${productId}/queue`
      : state === 'invited'
        ? `/products/${productId}/reservation`
        : state === 'checkout'
          ? `/products/${productId}/checkout`
          : `/products/${productId}/result`,
  useActiveQueueAttemptsQuery: () => ({ data: activeAttempts }),
}));

const { useQueueAttemptNotifications } = await import('./use-queue-attempt-notifications');

const productId = '11111111-1111-4111-8111-111111111111';
const attemptId = '22222222-2222-4222-8222-222222222222';

const createAttempt = (
  state: QueueAttemptState,
  overrides: Partial<QueueAttempt> = {},
): QueueAttempt => ({
  attempt_id: attemptId,
  created_at: '2026-08-07T10:00:00Z',
  message_code: `attempt_${state}`,
  next_action: 'wait',
  product_id: productId,
  queue_sequence: 1,
  state,
  updated_at: '2026-08-07T10:00:00Z',
  ...overrides,
});

function NotificationHarness() {
  useQueueAttemptNotifications();

  const location = useLocation();

  return <div data-testid="location">{location.pathname}</div>;
}

const renderNotifications = (initialEntry = '/') =>
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <NotificationHarness />
    </MemoryRouter>,
  );

describe('useQueueAttemptNotifications', () => {
  beforeEach(() => {
    activeAttempts = undefined;
    userId = '00000000-0000-4000-8000-000000000001';
    getQueueAttemptMock.mockReset();
    hideNotificationMock.mockReset();
    showNotificationMock.mockReset();
    updateNotificationMock.mockReset();
  });

  it('does not notify for the initial active-attempt snapshot', () => {
    activeAttempts = [createAttempt('waiting', { position: 3 })];

    renderNotifications();

    expect(showNotificationMock).not.toHaveBeenCalled();
  });

  it('notifies when the queue position changes and keeps the popup open', () => {
    activeAttempts = [createAttempt('waiting', { position: 3 })];
    const view = renderNotifications();

    activeAttempts = [
      createAttempt('waiting', { position: 2, updated_at: '2026-08-07T10:01:00Z' }),
    ];
    view.rerender(
      <MemoryRouter>
        <NotificationHarness />
      </MemoryRouter>,
    );

    expect(showNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({
        autoClose: false,
        title: 'Очередь обновилась',
        withCloseButton: true,
      }),
    );
    const notification = showNotificationMock.mock.calls[0][0];
    render(<MemoryRouter>{notification.message as ReactNode}</MemoryRouter>);
    expect(screen.getByRole('link', { name: 'Открыть' })).toHaveAttribute(
      'href',
      `/products/${productId}/queue`,
    );
  });

  it('redirects to the reservation when purchase becomes available without showing a popup', async () => {
    activeAttempts = [createAttempt('waiting', { position: 1 })];
    const view = renderNotifications();

    activeAttempts = [createAttempt('invited', { invited_at: '2026-08-07T10:01:00Z' })];
    view.rerender(
      <MemoryRouter>
        <NotificationHarness />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByTestId('location')).toHaveTextContent(`/products/${productId}/reservation`);
    });
    expect(showNotificationMock).not.toHaveBeenCalled();
  });

  it('redirects to the terminal result when an attempt leaves the active list', async () => {
    activeAttempts = [createAttempt('invited')];
    getQueueAttemptMock.mockResolvedValue(createAttempt('invite_expired'));
    const view = renderNotifications();

    activeAttempts = [];
    view.rerender(
      <MemoryRouter>
        <NotificationHarness />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByTestId('location')).toHaveTextContent(`/products/${productId}/result`);
    });
    expect(getQueueAttemptMock).toHaveBeenCalledWith(productId, userId);
    expect(showNotificationMock).not.toHaveBeenCalled();
  });

  it('prioritizes an invitation over checkout and terminal transitions', async () => {
    const invitedProductId = '33333333-3333-4333-8333-333333333333';
    const invitedAttemptId = '44444444-4444-4444-8444-444444444444';
    const checkoutProductId = '55555555-5555-4555-8555-555555555555';
    const checkoutAttemptId = '66666666-6666-4666-8666-666666666666';
    const terminalProductId = '77777777-7777-4777-8777-777777777777';
    const terminalAttemptId = '88888888-8888-4888-8888-888888888888';
    activeAttempts = [
      createAttempt('waiting', { attempt_id: invitedAttemptId, product_id: invitedProductId }),
      createAttempt('waiting', { attempt_id: checkoutAttemptId, product_id: checkoutProductId }),
      createAttempt('waiting', { attempt_id: terminalAttemptId, product_id: terminalProductId }),
    ];
    const view = renderNotifications(`/products/${checkoutProductId}/queue`);

    getQueueAttemptMock.mockResolvedValue(
      createAttempt('sold_out', { attempt_id: terminalAttemptId, product_id: terminalProductId }),
    );
    activeAttempts = [
      createAttempt('invited', { attempt_id: invitedAttemptId, product_id: invitedProductId }),
      createAttempt('checkout', { attempt_id: checkoutAttemptId, product_id: checkoutProductId }),
    ];
    view.rerender(
      <MemoryRouter>
        <NotificationHarness />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByTestId('location')).toHaveTextContent(
        `/products/${invitedProductId}/reservation`,
      );
    });
    await Promise.resolve();
    expect(screen.getByTestId('location')).toHaveTextContent(
      `/products/${invitedProductId}/reservation`,
    );
  });

  it('resets the snapshot when the active demo user changes', () => {
    activeAttempts = [createAttempt('waiting', { position: 3 })];
    const view = renderNotifications();

    userId = '00000000-0000-4000-8000-000000000002';
    activeAttempts = [createAttempt('invited')];
    view.rerender(
      <MemoryRouter>
        <NotificationHarness />
      </MemoryRouter>,
    );

    expect(showNotificationMock).not.toHaveBeenCalled();
  });

  it('closes this user\'s displayed notifications when the demo user changes', () => {
    activeAttempts = [createAttempt('waiting', { position: 3 })];
    const view = renderNotifications();

    activeAttempts = [createAttempt('waiting', { position: 2 })];
    view.rerender(
      <MemoryRouter>
        <NotificationHarness />
      </MemoryRouter>,
    );

    userId = '00000000-0000-4000-8000-000000000002';
    view.rerender(
      <MemoryRouter>
        <NotificationHarness />
      </MemoryRouter>,
    );

    expect(hideNotificationMock).toHaveBeenCalledWith(`queue-attempt:${attemptId}`);
  });
});
