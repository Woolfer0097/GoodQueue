import { getQueueAttemptRoute } from './queue-attempt-route';

const productId = '11111111-1111-4111-8111-111111111111';

describe('getQueueAttemptRoute', () => {
  it.each([
    ['waiting', `/products/${productId}/queue`],
    ['invited', `/products/${productId}/reservation`],
    ['checkout', `/products/${productId}/checkout`],
    ['purchased', `/products/${productId}/result`],
    ['invite_expired', `/products/${productId}/result`],
    ['checkout_expired', `/products/${productId}/result`],
    ['payment_failed', `/products/${productId}/result`],
    ['cancelled', `/products/${productId}/result`],
    ['sold_out', `/products/${productId}/result`],
  ] as const)('maps %s to %s', (state, route) => {
    expect(getQueueAttemptRoute(productId, state)).toBe(route);
  });
});
