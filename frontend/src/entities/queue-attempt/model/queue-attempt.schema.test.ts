import {
  queueAttemptListSchema,
  queueAttemptSchema,
  queueAttemptStateSchema,
} from './queue-attempt.schema';

const queueAttemptResponse = {
  attempt_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
  created_at: '2026-08-07T10:00:00Z',
  message_code: 'queue_waiting',
  next_action: 'wait',
  position: 1,
  position_ahead: 0,
  product_id: '11111111-1111-1111-1111-111111111111',
  queue_sequence: 2,
  state: 'waiting',
  total_waiting: 2,
  updated_at: '2026-08-07T10:00:01Z',
};

describe('queue attempt schemas', () => {
  it('contains only states exposed by the backend contract', () => {
    expect(queueAttemptStateSchema.options).toEqual([
      'waiting',
      'invited',
      'checkout',
      'purchased',
      'invite_expired',
      'checkout_expired',
      'payment_failed',
      'cancelled',
      'sold_out',
    ]);
  });

  it('parses a waiting attempt with queue position data', () => {
    expect(queueAttemptSchema.parse(queueAttemptResponse)).toEqual(queueAttemptResponse);
  });

  it('parses a list of active attempts', () => {
    expect(queueAttemptListSchema.parse([queueAttemptResponse])).toEqual([queueAttemptResponse]);
  });

  it('parses an invited attempt with backend deadlines', () => {
    const invitedAttempt = {
      ...queueAttemptResponse,
      expires_at: '2026-08-07T10:05:00Z',
      invited_at: '2026-08-07T10:01:00Z',
      message_code: 'checkout_available',
      next_action: 'start_checkout',
      state: 'invited',
    };

    expect(queueAttemptSchema.parse(invitedAttempt)).toEqual(invitedAttempt);
  });

  it.each([
    ['unknown state', { ...queueAttemptResponse, state: 'queue_disabled' }],
    ['invalid attempt ID', { ...queueAttemptResponse, attempt_id: 'attempt-1' }],
    ['negative position', { ...queueAttemptResponse, position: -1 }],
    ['invalid timestamp', { ...queueAttemptResponse, updated_at: 'today' }],
  ])('rejects %s', (_caseName, response) => {
    expect(() => queueAttemptSchema.parse(response)).toThrow();
  });
});
