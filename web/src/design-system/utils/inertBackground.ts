import { useCallback } from 'react';
import type { Ref } from 'react';

type InertScope = { users: number; release: () => void };
const scopes = new WeakMap<Document, InertScope>();

/**
 * Radix marks the inactive background for assistive technology. Give those same
 * branches native interaction isolation too: menus and listboxes cannot use aria-modal.
 * Follow its markers so nested portals and live regions keep Radix's boundaries.
 */
function acquireInertBackground(document: Document) {
  let scope = scopes.get(document);
  if (!scope) {
    const previous = new Map<Element, string | null>();
    const restore = (element: Element, value: string | null) => {
      if (value === null) element.removeAttribute('inert');
      else element.setAttribute('inert', value);
    };
    const synchronize = () => {
      const hidden = new Set(document.querySelectorAll('[data-aria-hidden="true"]'));
      for (const [element, value] of previous) {
        if (!hidden.has(element)) {
          restore(element, value);
          previous.delete(element);
        }
      }
      for (const element of hidden) {
        if (!previous.has(element)) {
          previous.set(element, element.getAttribute('inert'));
          element.setAttribute('inert', '');
        }
      }
    };
    const observer = new MutationObserver(synchronize);
    observer.observe(document.body, {
      subtree: true,
      childList: true,
      attributes: true,
      attributeFilter: ['data-aria-hidden'],
    });
    synchronize();
    scope = {
      users: 0,
      release: () => {
        observer.disconnect();
        for (const [element, value] of previous) restore(element, value);
      },
    };
    scopes.set(document, scope);
  }
  const current = scope;
  current.users += 1;
  return () => {
    current.users -= 1;
    if (current.users === 0) {
      current.release();
      scopes.delete(document);
    }
  };
}

export function useInertBackgroundRef<T extends HTMLElement>(ref?: Ref<T>) {
  return useCallback((node: T | null) => {
    const release = node ? acquireInertBackground(node.ownerDocument) : undefined;
    const refCleanup = typeof ref === 'function' ? ref(node) : undefined;
    if (ref && typeof ref !== 'function') ref.current = node;
    return () => {
      release?.();
      if (typeof refCleanup === 'function') refCleanup();
      else if (typeof ref === 'function') ref(null);
      else if (ref) ref.current = null;
    };
  }, [ref]);
}
