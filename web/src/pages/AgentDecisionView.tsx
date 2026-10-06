import { useId, type FormEvent, type Ref } from 'react';
import { CircleAlert, CircleX } from 'lucide-react';
import { Link } from 'react-router-dom';
import { DecisionSuccessCheck } from '@design-system/components/feedback/DecisionSuccessCheck/DecisionSuccessCheck';
import { Alert } from '@design-system/components/feedback/Alert';
import { Skeleton } from '@design-system/components/feedback/Skeleton';
import { Button } from '@design-system/components/primitives/Button';
import { accessCopy, commonCopy, navigationCopy } from '@copy';
import { consentCopy } from '@copy/consent';
import { DecisionIdentity } from '@components/consent/DecisionIdentity';
import { PermissionPanel } from '@components/consent/PermissionPanel';
import { DurationSelect } from '@components/consent/DurationSelect';
import { TechnicalDetails } from '@components/consent/TechnicalDetails';
import type { ConsentDraft } from '@components/consent/consentDraft';
import type { AgentDetailData } from '@services/api/consent';
import type { UserInfo } from '@app-types/consent';
import './AgentDecisionView.css';

export type AgentDecisionViewProps = { user: UserInfo } & (
  | { state: 'loading' }
  | { state: 'invalid'; message?: string }
  | { state: 'error'; onRetry: () => void }
  | { state: 'denied' }
  | {
    state: 'ready';
    allowed?: boolean;
    data: AgentDetailData;
    draft: ConsentDraft;
    pending: boolean;
    missingServiceIds: readonly string[];
    error?: string;
    selectionError?: string;
    errorGroupId?: string;
    errorRef?: Ref<HTMLDivElement>;
    dateError?: string;
    onChange: (draft: ConsentDraft) => void;
    onAllow: (event: FormEvent<HTMLFormElement>) => void;
    onDeny: () => void;
    onConnect: (serviceId: string) => void;
  }
);

const frame = 'flex max-h-[calc(100dvh-7rem)] w-full min-w-0 flex-col overflow-hidden rounded-2xl bg-card max-sm:max-h-[calc(100dvh-6.5rem)] max-sm:rounded-none';
const borderedFrame = `${frame} border border-border-subtle max-sm:border-x-0 max-sm:border-b-0`;

export function AgentDecisionView(props: AgentDecisionViewProps) {
  return <>
    <DecisionCard {...props} />
    <p data-testid="consent-account" className="mx-auto mt-3 max-w-full shrink-0 px-4 pb-1 text-center text-xs text-muted-foreground [overflow-wrap:anywhere] max-sm:pb-[max(0.75rem,env(safe-area-inset-bottom))]">
      {consentCopy.signedInAs(props.user.displayName || props.user.principal)}
    </p>
  </>;
}

function DecisionCard(props: AgentDecisionViewProps) {
  const connectionHintId = useId();
  if (props.state === 'loading') return <section data-testid="consent-card" className={`${borderedFrame} gap-5 p-6`} aria-busy="true">
    <Skeleton label={commonCopy.loading} width="75%" />
    <Skeleton width="55%" />
    <Skeleton variant="rounded" className="min-h-48 flex-1" />
    <Skeleton width="100%" height="2.5rem" />
  </section>;

  if (props.state === 'invalid' || props.state === 'error') return <section data-testid="consent-card" className={`${borderedFrame} items-center justify-center gap-4 p-6 text-center`}>
    <CircleAlert aria-hidden="true" className="size-10 text-status-danger-foreground" />
    <Alert hideIcon variant="error" data-testid="consent-error" className="max-w-sm text-left" action={props.state === 'error' ? { label: commonCopy.retry, onClick: props.onRetry } : undefined}>
      {props.state === 'invalid' ? props.message ?? consentCopy.invalidSession : consentCopy.loadError}
    </Alert>
  </section>;

  if (props.state === 'denied') return <section data-testid="consent-card" className={`${borderedFrame} items-center justify-center gap-4 p-6 text-center`}>
    <CircleX aria-hidden="true" className="size-12 rounded-full bg-muted p-2 text-muted-foreground" />
    <div data-testid="consent-outcome" role="status" className="space-y-2">
      <h1 className="font-display text-xl font-semibold">{consentCopy.deniedTitle}</h1>
      <p className="text-sm text-muted-foreground">{consentCopy.closeTab}</p>
    </div>
  </section>;

  const { agent, services, cimd_metadata: metadata } = props.data;
  const blocked = props.missingServiceIds.length > 0;
  return <form data-testid="consent-card" className={`${frame} consent-decision-surface shadow-focal ring-1 ring-border-subtle max-sm:h-[calc(100dvh-6.5rem)] max-sm:border-t max-sm:border-border-subtle max-sm:ring-0 max-sm:shadow-none`} onSubmit={props.onAllow} onInvalid={(event) => { (event.target as HTMLElement).focus(); }}>
    <div className="shrink-0 px-6 pb-3 pt-4 max-sm:px-4"><DecisionIdentity agent={agent} metadata={metadata} /></div>
    {props.error && <Alert ref={props.errorRef} tabIndex={-1} variant="error" data-testid="consent-error" className="mx-6 mb-2 max-sm:mx-4">{props.error}</Alert>}
    <div className="flex min-h-0 flex-1 flex-col px-6 pb-3 max-sm:px-4">
      <PermissionPanel draft={props.draft} services={services} disabled={props.pending || props.allowed} onChange={props.onChange} onConnect={props.onConnect} missingServiceIds={props.missingServiceIds} error={props.selectionError} errorGroupId={props.errorGroupId} />
    </div>
    <footer data-testid="consent-footer" className="sticky bottom-0 z-10 shrink-0 space-y-3 border-t border-border-subtle bg-card px-6 pb-5 pt-4 max-sm:px-4 max-sm:pb-[max(1rem,env(safe-area-inset-bottom))]">
      <DurationSelect draft={props.draft} disabled={props.pending || props.allowed} error={props.dateError} onChange={props.onChange} />
      {blocked && !props.allowed && <p id={connectionHintId} role="status" className="text-xs text-muted-foreground">{consentCopy.connectionRequired}</p>}
      {(props.selectionError || props.dateError) && <p role="status" data-testid="consent-validation-summary" className="text-xs text-status-danger-foreground">{consentCopy.validationSummary}</p>}
      <div className="flex gap-3">
        <Button type="button" variant="outline" className="min-w-0 flex-1" disabled={props.pending || props.allowed} onClick={props.onDeny}>{accessCopy.deny}</Button>
        {props.allowed ? <div data-testid="consent-outcome" role="status" className="flex h-10 min-w-0 flex-1 items-center justify-center rounded-lg bg-success text-success-foreground">
          <DecisionSuccessCheck className="size-6" /><span className="sr-only">{consentCopy.allowedTitle}</span>
        </div> : <Button type="submit" variant="primary" className="min-w-0 flex-1" disabled={blocked} aria-describedby={blocked ? connectionHintId : undefined} isLoading={props.pending}>{accessCopy.allow}</Button>}
      </div>
      <div className="flex flex-wrap items-baseline justify-between gap-x-2 gap-y-1 text-xs text-muted-foreground">
        <p data-testid="consent-next-steps">{consentCopy.reassurance} <Link to="/agents" className="underline underline-offset-4">{navigationCopy.agents}</Link>.</p>
        <TechnicalDetails agent={agent} metadata={metadata} />
      </div>
    </footer>
  </form>;
}
