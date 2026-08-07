const { createJoinQueueIdempotencyKey } = await import('./create-join-queue-idempotency-key');

describe('createJoinQueueIdempotencyKey', () => {
  it('creates a fresh scoped key for each new join intention', () => {
    const firstKey = createJoinQueueIdempotencyKey();
    const secondKey = createJoinQueueIdempotencyKey();

    expect(firstKey).not.toBe(secondKey);
    expect(firstKey.length).toBeGreaterThan(0);
    expect(secondKey.length).toBeLessThanOrEqual(128);
  });
});
