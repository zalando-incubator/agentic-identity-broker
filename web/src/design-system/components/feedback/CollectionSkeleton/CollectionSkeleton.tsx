import { Skeleton } from '@design-system/components/feedback/Skeleton';

export interface CollectionSkeletonProps {
  label: string;
  view?: 'grid' | 'list';
  cardSize?: 'default' | 'compact';
  count?: number;
}

export function CollectionSkeleton({ label, view = 'grid', cardSize = 'default', count = 6 }: CollectionSkeletonProps) {
  return <div role="status" aria-label={label} data-slot="collection-skeleton"
    className={view === 'grid' ? 'grid grid-cols-[repeat(auto-fill,minmax(min(100%,320px),1fr))] gap-4' : 'flex flex-col gap-3'}>
    <span className="sr-only">{label}</span>
    {Array.from({ length: Math.max(0, count) }, (_, index) => view === 'grid' && cardSize !== 'compact' ?
      <div key={index} aria-hidden="true" className="flex min-h-40 flex-col rounded-xl border border-border-subtle bg-card">
        <div className="flex items-start gap-3 px-4 pt-4">
          <Skeleton variant="rounded" className="h-10 w-10 shrink-0" />
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <Skeleton className="h-5 w-3/4" />
            <Skeleton variant="rounded" className="h-5 w-24" />
          </div>
        </div>
        <div className="mt-2 flex items-center gap-3 px-4">
          <Skeleton className="h-3 w-16" />
          <Skeleton className="h-3 w-28" />
        </div>
        <div className="mx-4 mt-auto flex justify-end gap-2 border-t border-border-subtle pb-3 pt-2">
          <Skeleton variant="rounded" className="h-8 w-16" />
          <Skeleton variant="rounded" className="h-8 w-24" />
        </div>
      </div>
      : view === 'grid' ? <div key={index} aria-hidden="true" className="flex h-[120px] flex-col rounded-xl border border-border-subtle bg-card p-3 max-[240px]:h-auto max-[240px]:min-h-[120px]">
        <div className="flex gap-3">
          <Skeleton variant="rounded" className="h-10 w-10 shrink-0 max-[240px]:hidden" />
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <Skeleton className="w-3/4" />
            <Skeleton className="h-3 w-1/2" />
            <Skeleton className="h-3 w-2/3" />
          </div>
        </div>
        <div className="mt-auto flex flex-wrap justify-end gap-2 max-[240px]:justify-start max-[240px]:gap-y-2">
          <Skeleton variant="rounded" className="h-8 w-20" />
          <Skeleton variant="rounded" className="h-8 w-24" />
        </div>
      </div>
      : cardSize === 'compact' ? <div key={index} aria-hidden="true" className="flex min-h-[88px] flex-col gap-2 rounded-xl border border-border-subtle bg-card p-3 md:flex-row md:items-center md:gap-3">
        <div className="flex min-w-0 flex-1 items-center gap-3"><Skeleton variant="rounded" className="h-10 w-10 shrink-0" /><Skeleton className="w-3/5" /></div>
        <Skeleton className="h-3 w-1/2 md:w-1/5" />
        <Skeleton className="h-3 w-2/3 md:w-1/5" />
        <div className="flex flex-wrap justify-end gap-2"><Skeleton variant="rounded" className="h-8 w-20" /><Skeleton variant="rounded" className="h-8 w-24" /></div>
      </div>
      : <div key={index} aria-hidden="true" className="flex h-[88px] items-center gap-3 rounded-xl border border-border-subtle bg-card p-3">
        <Skeleton variant="rounded" className="h-10 w-10 shrink-0" />
        <div className="flex min-w-0 flex-1 flex-col gap-2"><Skeleton className="w-3/5" /><Skeleton className="w-2/5" /></div>
        <Skeleton variant="rounded" className="h-8 w-16 shrink-0" />
      </div>)}
  </div>;
}
