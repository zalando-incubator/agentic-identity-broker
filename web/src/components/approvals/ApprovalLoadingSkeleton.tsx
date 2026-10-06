import { Skeleton } from '@design-system/components/feedback/Skeleton';
import { commonCopy } from '@copy';

export function ApprovalLoadingSkeleton() {
  return <div className="space-y-6" aria-busy="true" role="status" aria-label={commonCopy.loading}>
    <Skeleton width="60%" height="2rem" />
    <Skeleton width="80%" />
    <Skeleton height="14rem" variant="rounded" />
    <div className="flex gap-3"><Skeleton width="8rem" height="2.5rem" /><Skeleton width="8rem" height="2.5rem" /></div>
  </div>;
}
