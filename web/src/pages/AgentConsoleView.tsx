import { ArrowLeft, ArrowUpRight } from 'lucide-react';
import { formatDistanceStrict } from 'date-fns';
import type { FormEventHandler, Ref } from 'react';
import { Link } from 'react-router-dom';
import { accessCopy, commonCopy } from '@copy';
import { consentCopy } from '@copy/consent';
import { delegationsCopy } from '@copy/delegations';
import { AgentDetailSkeleton } from '@components/consent/AgentDetailSkeleton';
import { AgentConnectionsRail, type AgentConnection } from '@components/consent/AgentConnectionsRail';
import { GrantEditBar } from '@components/consent/GrantEditBar';
import { PermissionPanel } from '@components/consent/PermissionPanel';
import { DurationSelect } from '@components/consent/DurationSelect';
import { resolveValidUntil, type ConsentDraft } from '@components/consent/consentDraft';
import { RevokeAgentDialog } from '@components/consent/RevokeAgentDialog';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { Card, CardContent, CardHeader, CardTitle } from '@design-system/components/data-display/Card';
import { Alert } from '@design-system/components/feedback/Alert';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Button } from '@design-system/components/primitives/Button';
import type { AgentDetailData } from '@services/api/consent';
import type { AgentDetail, UserGrant } from '@app-types/consent';

export type AgentConsoleViewProps = { state: 'loading'; agentId?: string } | { state: 'error'; onRetry?: () => void } | {
  state: 'ready';
  data: AgentDetailData;
  draft: ConsentDraft;
  savedGrant: UserGrant | null;
  stale?: boolean;
  onRetry: () => void;
  error?: string;
  dateError?: string;
  pending: boolean;
  blocked: boolean;
  onSubmit: FormEventHandler<HTMLFormElement>;
  onDraftChange: (draft: ConsentDraft) => void;
  onDurationChange: (draft: ConsentDraft) => void;
  onCancel: () => void;
  connections: readonly AgentConnection[];
  connectionLoading: boolean;
  connectionError: boolean;
  connectionStorageError: boolean;
  onConnectionRetry: () => void;
  onConnect: (serviceId: string) => void;
  onRevoke: () => void;
  revokeConfirmation: boolean;
  revokePending: boolean;
  revokeError?: string;
  onCancelRevoke: () => void;
  onConfirmRevoke: () => void | Promise<void>;
  onReturnRevokeFocus: () => void;
  headingRef?: Ref<HTMLDivElement>;
  errorRef?: Ref<HTMLDivElement>;
  revokeRef?: Ref<HTMLButtonElement>;
};

/** Pure console composition: the container owns every read, mutation and callback. */
export function AgentConsoleView(props: AgentConsoleViewProps) {
  if (props.state === 'loading') return <AgentDetailSkeleton agentId={props.agentId} />;
  if (props.state === 'error') return <Alert variant="error" action={props.onRetry ? { label: commonCopy.retry, onClick: props.onRetry } : undefined}>{consentCopy.loadError}</Alert>;
  const { data, draft, savedGrant, stale, onRetry, error, dateError, pending, blocked,
    onSubmit, onDraftChange, onDurationChange, onCancel, connections, connectionLoading, connectionError, connectionStorageError,
    onConnectionRetry, onConnect, onRevoke, revokeConfirmation, revokePending, revokeError, onCancelRevoke, onConfirmRevoke,
    onReturnRevokeFocus, headingRef, errorRef, revokeRef } = props;

  const { agent, services } = data;
  const canRevoke = Boolean(savedGrant && Object.keys(savedGrant.granted_permission_sets).length > 0);
  const links = [
    { href: agent.governanceUrl, label: consentCopy.governance },
    { href: agent.userDocumentationUrl, label: consentCopy.documentation },
    { href: agent.agentInterfaceUrl, label: consentCopy.agentInterface },
  ].flatMap(({ href, label }) => {
    if (!href) return [];
    try {
      const url = new URL(href, window.location.origin);
      return url.protocol === 'http:' || url.protocol === 'https:' ? [{ href: url.toString(), label }] : [];
    } catch { return []; }
  });

  return <form onSubmit={onSubmit} onInvalid={(event) => { (event.target as HTMLElement).focus(); }} className="space-y-6">
    <header ref={headingRef} tabIndex={-1} className="space-y-4 outline-none focus-visible:ring-2 focus-visible:ring-ring">
      <Link to="/agents" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground hover:underline"><ArrowLeft aria-hidden="true" className="size-4" />{delegationsCopy.back}</Link>
      <div className="flex flex-wrap items-start gap-3" style={{ viewTransitionName: `entity-${agent.agentId}` }}>
        <Avatar id={agent.agentId} label={agent.displayName} aria-hidden="true" />
        <div className="min-w-0 flex-1 basis-48 sm:basis-0">
          <TruncatedText as="h1" text={agent.displayName} lines={1} data-testid="agent-name-heading" expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} className="font-display text-xl font-semibold" />
          <AgentConsoleMetadata grant={savedGrant} />
        </div>
        {canRevoke && <Button ref={revokeRef} type="button" variant="destructive-outline" disabled={pending || revokePending} onClick={onRevoke}>{delegationsCopy.revokeAccess}</Button>}
      </div>
    </header>
    {stale && <Alert variant="warning" action={{ label: commonCopy.retry, onClick: onRetry }}>{delegationsCopy.detailStale}</Alert>}
    {error && <Alert ref={errorRef} tabIndex={-1} variant="error">{error}</Alert>}
    {revokeError && <Alert variant="error">{revokeError}</Alert>}
    {draft.dirty && <GrantEditBar pending={pending} blocked={blocked} onCancel={onCancel} />}
    <div className="console-grid">
      <div className="col-span-12 min-w-0 space-y-6 min-[1012px]:col-span-8">
        <PermissionPanel draft={draft} services={services} mode="console" disabled={pending} onChange={onDraftChange} />
        <section className="rounded-xl border border-border-subtle bg-card px-4 py-3">
          <DurationSelect draft={draft} disabled={pending} error={dateError} onChange={onDurationChange} />
          <GrantExpiryPreview draft={draft} grant={savedGrant} />
        </section>
      </div>
      <aside className="col-span-12 min-w-0 space-y-6 min-[1012px]:col-span-4" aria-label={delegationsCopy.details}>
        <AgentConnectionsRail connections={connections} loading={connectionLoading} error={connectionError} storageError={connectionStorageError} disabled={pending} onRetry={onConnectionRetry} onConnect={onConnect} />
        {(agent.description || links.length > 0) && <section data-testid="agent-about" aria-label={delegationsCopy.about} className="rounded-xl border border-border-subtle bg-card p-4">
          <h2 className="font-display text-base font-semibold">{delegationsCopy.about}</h2>
          {agent.description && <TruncatedText as="p" text={agent.description} lines={3} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} className="mt-3 max-w-[70ch] text-sm text-muted-foreground" />}
          {links.length > 0 && <ul className="mt-3 space-y-2">{links.map(({ href, label }) => <li key={label}><a className="inline-flex items-center gap-1 text-sm text-primary hover:underline" href={href} target="_blank" rel="noopener noreferrer">{label}<ArrowUpRight aria-hidden="true" className="size-4" /></a></li>)}</ul>}
        </section>}
        <AgentConsoleTechnicalDetails agent={agent} grant={savedGrant} />
      </aside>
    </div>
    <RevokeAgentDialog open={revokeConfirmation} agentName={agent.displayName} pending={revokePending} onCancel={onCancelRevoke} onConfirm={onConfirmRevoke} onReturnFocus={onReturnRevokeFocus} />
  </form>;
}

const dateFormatter = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' });

function AgentConsoleMetadata({ grant }: { grant: UserGrant | null }) {
  if (!grant) return null;
  return <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs tabular-nums text-muted-foreground [overflow-wrap:anywhere]">
    <span>{grant.valid_until ? <>{delegationsCopy.accessUntil} <time dateTime={grant.valid_until}>{dateFormatter.format(new Date(grant.valid_until))}</time></> : accessCopy.untilRevoked}</span>
    <span>{delegationsCopy.changed} <time dateTime={grant.updated_at} title={dateFormatter.format(new Date(grant.updated_at))}>{formatDistanceStrict(new Date(grant.updated_at), Date.now(), { addSuffix: true })}</time></span>
  </div>;
}

function GrantExpiryPreview({ draft, grant }: { draft: ConsentDraft; grant: UserGrant | null }) {
  const initial = draft.reset();
  if (draft.duration === initial.duration && draft.customDate === initial.customDate) return null;
  let nextExpiry: string;
  try {
    const timestamp = resolveValidUntil(draft.duration, draft.customDate, new Date());
    nextExpiry = timestamp ? consentCopy.expiryOn(dateFormatter.format(new Date(timestamp))) : accessCopy.untilRevoked;
  } catch {
    nextExpiry = consentCopy.expiryPending;
  }
  return <dl aria-live="polite" className="mt-3 grid gap-1 border-t border-border-subtle pt-3 text-sm">
    <div className="flex flex-wrap gap-x-2"><dt className="text-muted-foreground">{consentCopy.currentExpiry}:</dt><dd>{grant?.valid_until ? consentCopy.expiryOn(dateFormatter.format(new Date(grant.valid_until))) : accessCopy.untilRevoked}</dd></div>
    <div className="flex flex-wrap gap-x-2"><dt className="font-medium">{consentCopy.pendingExpiry}:</dt><dd className="font-medium">{nextExpiry}</dd></div>
  </dl>;
}

function AgentConsoleTechnicalDetails({ agent, grant }: { agent: AgentDetail; grant: UserGrant | null }) {
  return <Card role="region" aria-label={consentCopy.technicalDetails} data-testid="agent-technical-details">
    <CardHeader><CardTitle>{consentCopy.technicalDetails}</CardTitle></CardHeader>
    <CardContent><dl className="space-y-3 text-sm">
      <div className="min-w-0"><dt className="text-muted-foreground">{consentCopy.agentId}</dt><dd className="break-all font-mono text-xs">{agent.agentId}</dd></div>
      {agent.clientId && <div className="min-w-0"><dt className="text-muted-foreground">{consentCopy.clientId}</dt><dd className="break-all font-mono text-xs">{agent.clientId}</dd></div>}
      {agent.clientUris && agent.clientUris.length > 0 && <div className="min-w-0"><dt className="text-muted-foreground">{consentCopy.clientUris}</dt><dd className="min-w-0 space-y-1 font-mono text-xs">
        {agent.clientUris.map((uri, index) => <span key={`${uri}-${index}`} className="block break-all">{uri}</span>)}
      </dd></div>}
      {grant && <>
        <div className="min-w-0"><dt className="text-muted-foreground">{consentCopy.grantId}</dt><dd className="break-all font-mono text-xs">{grant.id}</dd></div>
        <div className="min-w-0"><dt className="text-muted-foreground">{consentCopy.grantCreated}</dt><dd className="break-all font-mono text-xs"><time dateTime={grant.created_at}>{grant.created_at}</time></dd></div>
        <div className="min-w-0"><dt className="text-muted-foreground">{consentCopy.grantUpdated}</dt><dd className="break-all font-mono text-xs"><time dateTime={grant.updated_at}>{grant.updated_at}</time></dd></div>
      </>}
    </dl></CardContent>
  </Card>;
}
