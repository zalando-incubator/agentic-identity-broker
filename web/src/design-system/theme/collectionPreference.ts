export type CollectionView = 'grid' | 'list';
export type CollectionPage = 'agents' | 'connections';
export const DEFAULT_VIEW_STORAGE_KEY = 'aib.collection-view-default';
export const COLLECTION_PREFERENCE_EVENT = 'aib:collection-view-change';

const memory: Record<string, CollectionView | undefined> = {};
const memoryOnly = new Set<string>();
const collectionPages: CollectionPage[] = ['agents', 'connections'];

export function readCollectionPreference(page?: CollectionPage): CollectionView | undefined {
  const key = page ? `aib.collection-view.${page}` : DEFAULT_VIEW_STORAGE_KEY;
  if (!memoryOnly.has(key)) {
    try {
      const value = window.localStorage.getItem(key);
      memory[key] = value === 'grid' || value === 'list' ? value : undefined;
    } catch {
      // A blocked storage read keeps the last choice available in this tab.
    }
  }
  return memory[key];
}

export function writeCollectionPreference(view: CollectionView, page?: CollectionPage): void {
  const key = page ? `aib.collection-view.${page}` : DEFAULT_VIEW_STORAGE_KEY;
  memory[key] = view;
  try {
    window.localStorage.setItem(key, view);
    memoryOnly.delete(key);
  } catch {
    // A blocked write keeps this choice authoritative over any stale stored value.
    memoryOnly.add(key);
  }
  if (!page) {
    for (const collectionPage of collectionPages) {
      const pageKey = `aib.collection-view.${collectionPage}`;
      memory[pageKey] = undefined;
      try {
        window.localStorage.removeItem(pageKey);
        memoryOnly.delete(pageKey);
      } catch {
        // A blocked removal must not revive the former page choice on the next read.
        memoryOnly.add(pageKey);
      }
    }
  }
  window.dispatchEvent(new Event(COLLECTION_PREFERENCE_EVENT));
}

export function subscribeCollectionPreference(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(COLLECTION_PREFERENCE_EVENT, onChange);
  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(COLLECTION_PREFERENCE_EVENT, onChange);
  };
}
