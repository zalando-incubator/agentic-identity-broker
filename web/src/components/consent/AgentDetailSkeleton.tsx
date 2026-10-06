import { Skeleton } from '@design-system/components/feedback/Skeleton';
import { commonCopy } from '@copy';

export function AgentDetailSkeleton({ agentId }: { agentId?: string }) {
  return <div role="status" aria-label={commonCopy.loading} className="space-y-6" data-testid="agent-detail-skeleton">
    <div className="space-y-4">
      <Skeleton className="h-5 w-32" />
      <div className="flex items-center gap-3" style={{ viewTransitionName: agentId ? `entity-${agentId}` : undefined }}>
        <Skeleton variant="rounded" className="size-10" /><Skeleton className="h-7 w-52" />
      </div>
      <Skeleton className="h-10 max-w-[70ch]" />
    </div>
    <div className="console-grid">
      <div className="col-span-12 space-y-6 min-[1012px]:col-span-8"><Skeleton variant="rounded" className="h-56" /><Skeleton variant="rounded" className="h-12" /></div>
      <div className="col-span-12 space-y-6 min-[1012px]:col-span-4"><Skeleton variant="rounded" className="h-40" /><Skeleton variant="rounded" className="h-32" /><Skeleton variant="rounded" className="h-40" /></div>
    </div>
  </div>;
}
