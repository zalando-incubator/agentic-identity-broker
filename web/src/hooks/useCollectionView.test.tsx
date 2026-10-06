import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { subscribeCollectionPreference, writeCollectionPreference } from '@design-system/theme/collectionPreference';
import { useCollectionView, useDefaultCollectionView } from './useCollectionView';

beforeEach(() => localStorage.clear());
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  writeCollectionPreference('grid');
  localStorage.clear();
});

describe('collection layout preferences', () => {
  it('uses cards at twelve records and a compact list at thirteen without an explicit page choice', () => {
    const view = renderHook(({ count }) => useCollectionView('agents', count), { initialProps: { count: 12 } });
    expect(view.result.current.view).toBe('grid');
    view.rerender({ count: 13 });
    expect(view.result.current.view).toBe('list');
    view.rerender({ count: 12 });
    expect(view.result.current.view).toBe('grid');
  });
  it('prefers a page choice over the global preference and the global preference over density', () => {
    localStorage.setItem('aib.collection-view-default', 'grid');
    localStorage.setItem('aib.collection-view.agents', 'list');
    const agents = renderHook(() => useCollectionView('agents', 13));
    const connections = renderHook(() => useCollectionView('connections', 13));
    expect(agents.result.current.view).toBe('list');
    expect(connections.result.current.view).toBe('grid');
  });

  it('resets both mounted page choices with one notification and keeps later overrides local', () => {
    localStorage.setItem('aib.collection-view.agents', 'list');
    localStorage.setItem('aib.collection-view.connections', 'grid');
    const preference = renderHook(useDefaultCollectionView);
    const agents = renderHook(() => useCollectionView('agents', 13));
    const connections = renderHook(() => useCollectionView('connections', 13));
    const smallConnections = renderHook(() => useCollectionView('connections', 4));
    expect(agents.result.current.view).toBe('list');
    expect(connections.result.current.view).toBe('grid');
    expect(smallConnections.result.current.view).toBe('grid');

    const onChange = vi.fn();
    const unsubscribe = subscribeCollectionPreference(onChange);
    act(() => preference.result.current.setView('grid'));
    expect(onChange).toHaveBeenCalledTimes(1);
    unsubscribe();
    expect(agents.result.current.view).toBe('grid');
    expect(connections.result.current.view).toBe('grid');
    expect(localStorage.getItem('aib.collection-view.agents')).toBeNull();
    expect(localStorage.getItem('aib.collection-view.connections')).toBeNull();

    act(() => preference.result.current.setView('list'));
    expect(agents.result.current.view).toBe('list');
    expect(connections.result.current.view).toBe('list');
    expect(smallConnections.result.current.view).toBe('list');
    agents.unmount();
    connections.unmount();
    smallConnections.unmount();
    preference.unmount();

    const remountedPreference = renderHook(useDefaultCollectionView);
    const remountedAgents = renderHook(() => useCollectionView('agents', 13));
    const remountedConnections = renderHook(() => useCollectionView('connections', 13));
    expect(remountedPreference.result.current.view).toBe('list');
    expect(remountedAgents.result.current.view).toBe('list');
    expect(remountedConnections.result.current.view).toBe('list');
    act(() => remountedAgents.result.current.setView('grid'));
    expect(remountedAgents.result.current.view).toBe('grid');
    expect(remountedConnections.result.current.view).toBe('list');
    expect(remountedPreference.result.current.view).toBe('list');
    expect(localStorage.getItem('aib.collection-view.agents')).toBe('grid');
    expect(localStorage.getItem('aib.collection-view.connections')).toBeNull();
  });

  it('retains the global choice across remounts when reads fail but writes and removals succeed', () => {
    const preference = renderHook(useDefaultCollectionView);
    const agents = renderHook(() => useCollectionView('agents', 13));
    expect(agents.result.current.view).toBe('list');
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new DOMException('Blocked', 'SecurityError'); });
    act(() => preference.result.current.setView('grid'));
    expect(agents.result.current.view).toBe('grid');
    agents.unmount();
    preference.unmount();
    expect(renderHook(() => useCollectionView('agents', 13)).result.current.view).toBe('grid');
  });

  it('keeps a settings choice when writes fail but reads see stale storage', () => {
    localStorage.setItem('aib.collection-view-default', 'list');
    localStorage.setItem('aib.collection-view.agents', 'list');
    const preference = renderHook(useDefaultCollectionView);
    const agents = renderHook(() => useCollectionView('agents', 13));
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new DOMException('Blocked', 'SecurityError'); });
    act(() => preference.result.current.setView('grid'));
    expect(localStorage.getItem('aib.collection-view-default')).toBe('list');
    expect(localStorage.getItem('aib.collection-view.agents')).toBeNull();
    expect(preference.result.current.view).toBe('grid');
    expect(agents.result.current.view).toBe('grid');
    agents.unmount();
    preference.unmount();
    expect(renderHook(useDefaultCollectionView).result.current.view).toBe('grid');
    expect(renderHook(() => useCollectionView('agents', 13)).result.current.view).toBe('grid');
  });

  it('does not revive previous page choices when removals fail but reads and writes succeed', () => {
    localStorage.setItem('aib.collection-view.agents', 'list');
    localStorage.setItem('aib.collection-view.connections', 'list');
    const preference = renderHook(useDefaultCollectionView);
    const agents = renderHook(() => useCollectionView('agents', 13));
    const connections = renderHook(() => useCollectionView('connections', 13));
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => { throw new DOMException('Blocked', 'SecurityError'); });
    act(() => preference.result.current.setView('grid'));
    expect(localStorage.getItem('aib.collection-view.agents')).toBe('list');
    expect(localStorage.getItem('aib.collection-view.connections')).toBe('list');
    expect(agents.result.current.view).toBe('grid');
    expect(connections.result.current.view).toBe('grid');
    agents.unmount();
    connections.unmount();
    preference.unmount();
    const remountedAgents = renderHook(() => useCollectionView('agents', 13));
    const remountedConnections = renderHook(() => useCollectionView('connections', 13));
    expect(remountedAgents.result.current.view).toBe('grid');
    expect(remountedConnections.result.current.view).toBe('grid');
    act(() => remountedAgents.result.current.setView('list'));
    expect(remountedAgents.result.current.view).toBe('list');
    expect(remountedConnections.result.current.view).toBe('grid');
  });

  it('keeps settings and later page choices in memory when all storage operations are blocked', () => {
    localStorage.setItem('aib.collection-view.agents', 'list');
    localStorage.setItem('aib.collection-view.connections', 'grid');
    const preference = renderHook(useDefaultCollectionView);
    const agents = renderHook(() => useCollectionView('agents', 13));
    const connections = renderHook(() => useCollectionView('connections', 13));
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new DOMException('Blocked', 'SecurityError'); });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new DOMException('Blocked', 'SecurityError'); });
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => { throw new DOMException('Blocked', 'SecurityError'); });
    act(() => preference.result.current.setView('grid'));
    expect(agents.result.current.view).toBe('grid');
    expect(connections.result.current.view).toBe('grid');
    preference.unmount();
    agents.unmount();
    connections.unmount();
    const remountedPreference = renderHook(useDefaultCollectionView);
    const remountedAgents = renderHook(() => useCollectionView('agents', 13));
    const remountedConnections = renderHook(() => useCollectionView('connections', 13));
    expect(remountedPreference.result.current.view).toBe('grid');
    expect(remountedAgents.result.current.view).toBe('grid');
    expect(remountedConnections.result.current.view).toBe('grid');
    act(() => remountedConnections.result.current.setView('list'));
    expect(remountedAgents.result.current.view).toBe('grid');
    expect(remountedConnections.result.current.view).toBe('list');
  });

  it('keeps an explicitly changed layout across remounts when storage is blocked', () => {
    const normal = renderHook(() => useCollectionView('agents', 4));
    normal.unmount();
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new DOMException('Blocked', 'SecurityError'); });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new DOMException('Blocked', 'SecurityError'); });
    const blocked = renderHook(() => useCollectionView('agents', 4));
    act(() => blocked.result.current.setView('list'));
    expect(blocked.result.current.view).toBe('list');
    blocked.unmount();
    const remounted = renderHook(() => useCollectionView('agents', 4));
    expect(remounted.result.current.view).toBe('list');
  });
});
