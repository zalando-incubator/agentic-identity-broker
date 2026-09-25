/**
 * Vitest setup file.
 * Runs before all test files.
 */

import { expect, afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';
import * as matchers from '@testing-library/jest-dom/matchers';

// Extend Vitest's expect with jest-dom matchers
expect.extend(matchers);

class ResizeObserverMock {
  constructor(_callback: ResizeObserverCallback) {}

  observe(_target: Element) {}

  unobserve(_target: Element) {}

  disconnect() {}
}

if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = ResizeObserverMock;
}

// Cleanup after each test case
afterEach(() => {
  cleanup();
});
