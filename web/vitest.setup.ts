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

// Node's optional native storage must not shadow the browser storage in jsdom.
const jsdomWindow = (globalThis as typeof globalThis & {
  jsdom?: { window: typeof window };
}).jsdom?.window;
if (jsdomWindow) {
  Object.defineProperties(globalThis, {
    localStorage: { configurable: true, get: () => jsdomWindow.localStorage },
    sessionStorage: { configurable: true, get: () => jsdomWindow.sessionStorage },
    Storage: { configurable: true, writable: true, value: jsdomWindow.Storage },
  });
  const element = jsdomWindow.HTMLElement.prototype;
  if (!element.scrollIntoView) {
    Object.defineProperty(element, 'scrollIntoView', { configurable: true, writable: true, value() {} });
  }
  if (!element.hasPointerCapture) {
    const captures = new WeakMap<HTMLElement, Set<number>>();
    Object.defineProperties(element, {
      hasPointerCapture: {
        configurable: true,
        writable: true,
        value(this: HTMLElement, id: number) { return captures.get(this)?.has(id) ?? false; },
      },
      setPointerCapture: {
        configurable: true,
        writable: true,
        value(this: HTMLElement, id: number) {
          const pointers = captures.get(this) ?? new Set<number>();
          pointers.add(id);
          captures.set(this, pointers);
        },
      },
      releasePointerCapture: {
        configurable: true,
        writable: true,
        value(this: HTMLElement, id: number) { captures.get(this)?.delete(id); },
      },
    });
  }
}

// Cleanup after each test case
afterEach(() => {
  cleanup();
});
