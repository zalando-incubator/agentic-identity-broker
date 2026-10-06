import type { KeyboardEvent as ReactKeyboardEvent } from 'react';
import { AnimatePresence, motion, useIsPresent } from 'motion/react';
import { Link } from 'react-router-dom';
import { X } from 'lucide-react';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Badge } from '@design-system/components/primitives/Badge';
import { Button } from '@design-system/components/primitives/Button';
import { EntityRow } from '@design-system/components/data-display/Entity';
import { CollectionToolbar } from '@design-system/components/data-display/CollectionToolbar';
import { CollectionSkeleton } from '@design-system/components/feedback/CollectionSkeleton';
import { Skeleton } from '@design-system/components/feedback/Skeleton';
import { EmptyState } from '@design-system/components/feedback/EmptyState';
import { PageHeader } from '@design-system/components/layout/PageHeader';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@design-system/components/navigation/Tabs';
import { consoleMotion, useConsoleReducedMotion } from '@design-system/utils/consoleMotion';
import { Popover, PopoverContent, PopoverTrigger } from '@design-system/components/overlays/Popover';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@design-system/components/overlays/Dialog';
import { AnimatedCollection } from '@components/layout/AnimatedCollection';
import { AnimatedShieldCheckIcon } from '@components/icons/AnimatedIcons';
import { ApprovalReviewPage } from '@components/approvals/ApprovalReviewPage';
import type { ApprovalReviewPageProps } from '@components/approvals/ApprovalReviewPage';
import { PendingApprovalCount, usePendingCountBump } from '@components/approvals/PendingApprovalCount';
import { approvalTimeAgo } from '@components/approvals/approvalTimeAgo';
import { ApprovalDecisionIdentity } from '@components/approvals/ApprovalDecisionIdentity';
import { RiskBadge } from '@components/approvals/RiskBadge';
import { accessCopy, collectionCopy, commonCopy, navigationCopy } from '@copy';
import { approvalQueueCopy as copy } from '@copy/approvalQueue';
import type { ToolApprovalDetail } from '../types/approval';

export type RememberedFilter = 'all' | 'allowed' | 'denied';

const rememberedFilterOptions = [
  { value: 'all', label: copy.all },
  { value: 'allowed', label: copy.allowed },
  { value: 'denied', label: copy.denied },
] as const;
const rememberedToolbarLabels = { ...collectionCopy, search: copy.searchRemembered, filter: copy.filter };

interface ApprovalListState {
  data?: readonly ToolApprovalDetail[];
  loading: boolean;
  stale: boolean;
  onRetry: () => void;
}

export interface ApprovalsViewProps {
  tab: 'pending' | 'remembered';
  compact?: boolean;
  minRows?: number;
  pending: ApprovalListState;
  standing: ApprovalListState;
  selectedId: string | null;
  onSelect: (id: string) => void;
  onApproveOnce?: () => void;
  onDenyOnce?: () => void;
  review: ApprovalReviewPageProps | null;
  search: string;
  onSearchChange: (search: string) => void;
  filter: RememberedFilter;
  onFilterChange: (filter: RememberedFilter) => void;
  revoke: {
    confirmation: ToolApprovalDetail | null;
    isPending: (id: string) => boolean;
    error: boolean;
    request: (approval: ToolApprovalDetail, trigger: HTMLButtonElement) => void;
    cancel: () => void;
    confirm: () => void;
    returnFocus?: () => void;
  };
}

function matchesRemembered(approval: ToolApprovalDetail, filter: RememberedFilter, term: string): boolean {
  if (filter !== 'all' && approval.status !== (filter === 'allowed' ? 'approved' : 'denied')) return false;
  if (!term) return true;
  return approval.tool_name.toLocaleLowerCase().includes(term) ||
    (approval.agent_display_name?.trim() || approval.agent_id).toLocaleLowerCase().includes(term) ||
    approval.pattern_preview.toLocaleLowerCase().includes(term);
}

function pendingRows(approvals: readonly ToolApprovalDetail[], selectedId: string | null, onSelect: (id: string) => void, compact: boolean, reviewId: string | null, reduced: boolean) {
  return approvals.map(approval => {
    const agentName = approval.agent_display_name?.trim() || approval.agent_id;
    const remaining = Date.parse(approval.expires_at) - Date.now();
    const expiryMinutes = remaining > 0 && remaining < 60 * 60_000 ? Math.max(1, Math.ceil(remaining / 60_000)) : null;
    return <EntityRow key={approval.id} id={approval.id} name={approval.tool_name} nameClassName="font-mono" rowLayout="stacked"
      data-testid="pending-approval-row" role="listitem" selected={selectedId === approval.id}
      selectionIndicator={selectedId === approval.id && <motion.span aria-hidden="true" layoutId={reduced ? undefined : 'approval-selected-accent'}
        className="pointer-events-none absolute inset-y-2 left-0 w-1 rounded-r bg-primary" initial={{ opacity: 0 }} animate={{ opacity: 1 }}
        transition={{ duration: consoleMotion.feedback, ease: consoleMotion.ease }} />}
      aria-controls={!compact && selectedId === approval.id && reviewId === approval.id ? `approval-review-${approval.id}` : undefined}
      avatar={<Avatar id={approval.agent_id} label={agentName} />}
      supporting={<span data-testid="approval-agent-name" title={agentName}>{agentName}</span>}
      status={<RiskBadge level={approval.risk_level} />}
      meta={<span className="flex min-w-0 items-center gap-1">{expiryMinutes !== null && <span className="truncate" title={copy.expiresIn(expiryMinutes)}>{copy.expiresSoon(expiryMinutes)}</span>}<time className={expiryMinutes !== null ? 'sr-only' : 'truncate'} dateTime={approval.created_at} title={new Date(approval.created_at).toLocaleString()}>{approvalTimeAgo(approval.created_at)}</time></span>}
      href={compact ? `/approvals/${encodeURIComponent(approval.id)}` : undefined}
      onSelect={compact ? undefined : () => onSelect(approval.id)} />;
  });
}

function rememberedRows(approvals: readonly ToolApprovalDetail[], onRequest: ApprovalsViewProps['revoke']['request'], isPending: ApprovalsViewProps['revoke']['isPending']) {
  return approvals.map(approval => {
    const agentName = approval.agent_display_name?.trim() || approval.agent_id;
    return <EntityRow key={approval.id} id={approval.id} name={approval.tool_name} nameClassName="font-mono" rowLayout="stacked" data-testid="standing-decision-row" role="listitem"
      avatar={<Avatar id={approval.agent_id} label={agentName} />}
      supporting={<span data-testid="approval-agent-name" title={agentName}>{agentName}</span>}
      status={<Badge variant={approval.status === 'approved' ? 'success' : 'danger'} data-testid="approval-decision">{approval.status === 'approved' ? copy.allowed : copy.denied}</Badge>}
      meta={<Popover><PopoverTrigger asChild><Button variant="ghost" size="sm" aria-label={copy.viewPattern} className="h-auto min-w-0 max-w-full justify-start px-0 py-0 font-mono text-xs"><code data-testid="approval-scope-preview" className="block max-w-full truncate">{approval.pattern_preview}</code></Button></PopoverTrigger><PopoverContent align="start" aria-label={copy.viewPattern}><code className="block whitespace-pre-wrap break-all font-mono text-xs">{approval.pattern_preview}</code></PopoverContent></Popover>}
      actions={<Button variant="outline" size="sm" disabled={isPending(approval.id)} isLoading={isPending(approval.id)} onClick={event => onRequest(approval, event.currentTarget)}>{isPending(approval.id) ? copy.revoking : accessCopy.revoke}</Button>}
      busy={isPending(approval.id)} />;
  });
}

interface ShortcutContext {
  approvals: readonly ToolApprovalDetail[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  review: ApprovalReviewPageProps | null;
  onApproveOnce?: () => void;
  onDenyOnce?: () => void;
}

/** Inbox shortcuts require focus and an event target inside the pending panel. */
export function handleApprovalShortcut(event: ReactKeyboardEvent<HTMLElement>, context: ShortcutContext) {
  if (event.defaultPrevented || event.repeat || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;
  const target = event.target instanceof Element ? event.target : null;
  const inbox = event.currentTarget;
  const document = inbox.ownerDocument;
  if (!target || !inbox.contains(target) || !inbox.contains(document.activeElement)) return;
  if (target.closest('input,textarea,select,[contenteditable]:not([contenteditable="false"]),[role="combobox"],[inert],[aria-hidden="true"]')) return;
  if (document.querySelector('[role="dialog"],[role="menu"],[role="listbox"],[data-slot="popover-content"]')) return;
  if (inbox.querySelector('[data-active-approval] [data-testid="approval-review-panel"][data-editor-active="true"]')) return;
  const inRow = Boolean(target?.closest('[data-testid="pending-approval-row"]'));
  if (target?.closest('button,a,[role="tab"]') && !inRow) return;
  const index = context.approvals.findIndex(approval => approval.id === context.selectedId);
  if (event.key.toLowerCase() === 'j' || event.key.toLowerCase() === 'k') {
    if (!context.approvals.length || context.review?.submitting) return;
    const nextIndex = event.key.toLowerCase() === 'j' ? Math.min(index + 1, context.approvals.length - 1) : Math.max(index - 1, 0);
    context.onSelect(context.approvals[nextIndex].id);
    event.preventDefault();
    return;
  }
  if (event.key === 'Enter' && context.review) {
    inbox.querySelector<HTMLElement>('[data-active-approval] [data-testid="approval-review-panel"]')?.focus({ preventScroll: true });
    event.preventDefault();
    return;
  }
  const review = context.review;
  const selected = review?.approval;
  if (!review || !selected || selected.id !== context.selectedId || selected.status !== 'pending' || selected.principal !== review.actingPrincipal || review.submitting || review.approveResult || review.denyResult ||
    !Number.isFinite(Date.parse(selected.expires_at)) || Date.parse(selected.expires_at) <= Date.now() ||
    review.errorCode === 'EXPIRED' || review.errorCode === 'ALREADY_ACTIONED' || review.errorCode === 'FORBIDDEN' || review.errorCode === 'NOT_FOUND') return;
  if (event.key.toLowerCase() === 'a' && context.onApproveOnce) {
    event.preventDefault();
    context.onApproveOnce();
  } else if (event.key.toLowerCase() === 'd' && context.onDenyOnce) {
    event.preventDefault();
    context.onDenyOnce();
  }
}

function FadingApprovalDetail({ review, inboxSelected }: { review: ApprovalReviewPageProps; inboxSelected: boolean }) {
  const present = useIsPresent();
  return <motion.div id={`approval-review-${review.approval.id}`} data-active-approval={present || undefined} data-exiting={!present || undefined}
    inert={!present} aria-hidden={!present || undefined}
    className="min-w-0 data-[exiting=true]:pointer-events-none data-[exiting=true]:absolute data-[exiting=true]:inset-0"
    initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
    transition={{ duration: consoleMotion.feedback, ease: consoleMotion.ease }}>
    <ApprovalReviewPage {...review} inboxSelected={inboxSelected} decisionEnabled={present} />
  </motion.div>;
}

function PendingApprovalSkeleton() {
  return <div className="grid min-w-0 items-start gap-6 min-[1012px]:grid-cols-12">
    <div className="min-w-0 min-[1012px]:col-span-5"><CollectionSkeleton label={navigationCopy.pendingLoading} view="list" count={3} /></div>
    <div aria-hidden="true" data-testid="pending-review-skeleton" className="hidden h-[min(42rem,calc(100dvh-12rem))] min-h-80 min-w-0 flex-col overflow-hidden rounded-xl border border-border-subtle bg-card min-[1012px]:col-span-7 min-[1012px]:sticky min-[1012px]:top-6 min-[1012px]:flex">
      <div className="space-y-3 border-b border-border-subtle p-4"><Skeleton className="w-2/3" /><Skeleton className="w-1/2" /></div>
      <div className="min-h-0 flex-1 space-y-4 p-4"><Skeleton className="w-3/4" /><Skeleton variant="rounded" className="h-24 w-full" /><Skeleton className="w-2/3" /></div>
      <div className="flex justify-between gap-3 border-t border-border-subtle p-3"><Skeleton variant="rounded" className="h-10 w-24" /><Skeleton variant="rounded" className="h-10 w-32" /></div>
    </div>
  </div>;
}

type RememberedApprovalListProps = Pick<ApprovalsViewProps, 'standing' | 'search' | 'onSearchChange' | 'filter' | 'onFilterChange' | 'revoke'>;

function RememberedApprovalList({ standing, search, onSearchChange, filter, onFilterChange, revoke }: RememberedApprovalListProps) {
  const term = search.trim().toLocaleLowerCase();
  const visible = standing.data?.filter(approval => matchesRemembered(approval, filter, term));
  return <section className="min-w-0 space-y-4" aria-label={copy.rememberedTitle}>
    <div className="flex min-w-0 justify-end"><CollectionToolbar
      search={search} onSearchChange={onSearchChange}
      filters={rememberedFilterOptions} filter={filter}
      onFilterChange={value => onFilterChange(value === 'allowed' || value === 'denied' ? value : 'all')}
      labels={rememberedToolbarLabels} /></div>
    {filter !== 'all' && <div className="flex flex-wrap items-center gap-2">
      <Button variant="outline" size="sm" aria-label={`${collectionCopy.clearFilter}: ${filter === 'allowed' ? copy.allowed : copy.denied}`} onClick={() => onFilterChange('all')}><X aria-hidden="true" />{copy.filter}: {filter === 'allowed' ? copy.allowed : copy.denied}</Button>
    </div>}
    {revoke.error && <p role="alert" className="text-sm text-status-danger-foreground">{copy.revokeError}</p>}
    {standing.data ? visible?.length ? <div data-testid="standing-decisions" role="list" aria-label={copy.rememberedTitle}>
      <AnimatedCollection className="flex flex-col gap-3">{rememberedRows(visible, revoke.request, revoke.isPending)}</AnimatedCollection>
    </div> : standing.data.length ? <EmptyState size="compact" title={collectionCopy.noResults}>
      <div className="flex flex-wrap justify-center gap-3">
        {search && <Button variant="outline" onClick={() => onSearchChange('')}>{collectionCopy.clearSearch}</Button>}
        {filter !== 'all' && <Button variant="outline" onClick={() => onFilterChange('all')}>{collectionCopy.clearFilter}</Button>}
      </div>
    </EmptyState> : <EmptyState icon={<AnimatedShieldCheckIcon />} title={copy.rememberedEmpty} description={copy.rememberedEmptyDescription} /> : standing.loading && <CollectionSkeleton label={copy.standingLoading} view="list" count={3} />}
  </section>;
}

/** Network-free render of both console routes. Decision requests are only callbacks supplied by the container. */
export function ApprovalsView({ tab, compact = false, minRows = 0, pending, standing, selectedId, onSelect, onApproveOnce, onDenyOnce, review, search, onSearchChange, filter, onFilterChange, revoke }: ApprovalsViewProps) {
  const reduced = useConsoleReducedMotion();
  const countBump = usePendingCountBump(pending.data, pending.stale);
  const list = tab === 'pending' ? pending : standing;
  return <div className="min-w-0 space-y-5">
    <PageHeader title={navigationCopy.approvals} />
    <Tabs value={tab} className="gap-5"><TabsList aria-label={navigationCopy.approvals}>
      <TabsTrigger value="pending" asChild><Link to="/approvals">{copy.pendingTitle}{pending.data && <span className="tabular-nums">· <PendingApprovalCount count={pending.data.length} bumpRevision={countBump} /></span>}</Link></TabsTrigger>
      <TabsTrigger value="remembered" asChild><Link to="/approvals/remembered">{copy.rememberedTitle}</Link></TabsTrigger>
    </TabsList>
    {list.stale && <div role="alert" className="flex flex-wrap items-center gap-3 text-sm text-status-danger-foreground"><p>{list.data ? tab === 'pending' ? navigationCopy.pendingStale : copy.standingStale : tab === 'pending' ? copy.pendingError : copy.standingError}</p><Button variant="outline" size="sm" onClick={list.onRetry}>{commonCopy.retry}</Button></div>}
    <AnimatePresence initial={false}><motion.div key={tab} className="min-w-0" initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: consoleMotion.control, ease: consoleMotion.ease }}>
    <TabsContent value="pending" className="p-0" onKeyDown={compact ? undefined : event => handleApprovalShortcut(event, { approvals: pending.data ?? [], selectedId, onSelect, review, onApproveOnce, onDenyOnce })}><section data-testid="approval-section" aria-label={copy.pendingTitle} className="min-w-0">
      {pending.data ? pending.data.length ? <div className="grid min-w-0 items-start gap-6 min-[1012px]:grid-cols-12">
        <div className="min-w-0 min-[1012px]:col-span-5">
          <div data-testid="pending-approvals" role="list" aria-label={copy.pendingTitle} style={{ minHeight: compact || minRows === 0 ? undefined : minRows * 88 + Math.max(0, minRows - 1) * 12 }}>
            <AnimatedCollection className="flex flex-col gap-3">{pendingRows(pending.data, selectedId, onSelect, compact, review?.approval.id ?? null, reduced)}</AnimatedCollection>
          </div>
          {!compact && <p className="mt-3 hidden text-xs text-muted-foreground min-[1012px]:block">{copy.shortcutScope} {copy.nextShortcut} · {copy.previousShortcut} · {copy.focusShortcut}</p>}
        </div>
        <div className="relative hidden min-w-0 min-[1012px]:col-span-7 min-[1012px]:block min-[1012px]:sticky min-[1012px]:top-6">
          <AnimatePresence initial={false}>
            {review && <FadingApprovalDetail key={review.approval.id} review={review} inboxSelected={selectedId === review.approval.id} />}
          </AnimatePresence>
        </div>
      </div> : <EmptyState icon={<AnimatedShieldCheckIcon />} title={copy.pendingEmpty} description={copy.pendingEmptyDescription} /> : pending.loading && <PendingApprovalSkeleton />}
    </section></TabsContent><TabsContent value="remembered" className="p-0"><RememberedApprovalList standing={standing}
      search={search} onSearchChange={onSearchChange} filter={filter} onFilterChange={onFilterChange} revoke={revoke} /></TabsContent>
    </motion.div></AnimatePresence></Tabs>
    <Dialog open={revoke.confirmation !== null} onOpenChange={open => { if (!open) revoke.cancel(); }}>
      <DialogContent closeLabel={commonCopy.close} onCloseAutoFocus={event => { event.preventDefault(); revoke.returnFocus?.(); }}>
        <DialogHeader><DialogTitle>{copy.revokeTitle}</DialogTitle><DialogDescription asChild><div className="min-w-0 space-y-2"><p>{copy.revokeDescription}</p>
          {revoke.confirmation && <ApprovalDecisionIdentity toolName={revoke.confirmation.tool_name} agentName={revoke.confirmation.agent_display_name ?? revoke.confirmation.agent_id} />}
          <p>{copy.revokeEffect}</p>
        </div></DialogDescription></DialogHeader>
        <DialogFooter><Button variant="outline" onClick={revoke.cancel}>{commonCopy.cancel}</Button><Button variant="destructive" onClick={revoke.confirm}>{copy.confirmRevoke}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  </div>;
}
