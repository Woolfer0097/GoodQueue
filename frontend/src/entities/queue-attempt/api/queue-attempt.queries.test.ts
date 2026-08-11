import { queueAttemptQueryKeys } from './queue-attempt.query-keys';

describe('queue attempt query keys', () => {
  it('separates current attempts by product and selected user', () => {
    const productId = '11111111-1111-1111-1111-111111111111';
    const firstUserId = '00000000-0000-4000-8000-000000000001';
    const secondUserId = '00000000-0000-4000-8000-000000000002';

    expect(queueAttemptQueryKeys.current(productId, firstUserId)).toEqual([
      'queue-attempts',
      'current',
      productId,
      firstUserId,
    ]);
    expect(queueAttemptQueryKeys.current(productId, firstUserId)).not.toEqual(
      queueAttemptQueryKeys.current(productId, secondUserId),
    );
  });

  it('separates active attempt lists by selected user', () => {
    const firstUserId = '00000000-0000-4000-8000-000000000001';
    const secondUserId = '00000000-0000-4000-8000-000000000002';

    expect(queueAttemptQueryKeys.active(firstUserId)).toEqual([
      'queue-attempts',
      'active',
      firstUserId,
    ]);
    expect(queueAttemptQueryKeys.active(firstUserId)).not.toEqual(
      queueAttemptQueryKeys.active(secondUserId),
    );
  });
});
