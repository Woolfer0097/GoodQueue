import '@testing-library/jest-dom';

import { TextDecoder, TextEncoder } from 'node:util';

Object.assign(globalThis, { TextDecoder, TextEncoder });

class ResizeObserverMock {
  disconnect() {}

  observe() {}

  unobserve() {}
}

Object.assign(globalThis, { ResizeObserver: ResizeObserverMock });

Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
  configurable: true,
  value: () => undefined,
});
