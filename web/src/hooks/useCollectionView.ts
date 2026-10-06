import { useCallback, useSyncExternalStore } from 'react';
import {
  readCollectionPreference,
  writeCollectionPreference,
  subscribeCollectionPreference,
  type CollectionPage,
  type CollectionView,
} from '@design-system/theme/collectionPreference';

export function useCollectionView(page: CollectionPage, count: number) {
  const getSnapshot = useCallback(() => readCollectionPreference(page)
    ?? readCollectionPreference() ?? (count > 12 ? 'list' : 'grid'), [page, count]);
  const view = useSyncExternalStore(subscribeCollectionPreference, getSnapshot);
  const setView = useCallback((next: CollectionView) => writeCollectionPreference(next, page), [page]);
  return { view, setView };
}

export function useDefaultCollectionView() {
  const getSnapshot = useCallback(() => readCollectionPreference() ?? 'grid', []);
  const view = useSyncExternalStore(subscribeCollectionPreference, getSnapshot);
  const setView = useCallback((next: CollectionView) => writeCollectionPreference(next), []);
  return { view, setView };
}
