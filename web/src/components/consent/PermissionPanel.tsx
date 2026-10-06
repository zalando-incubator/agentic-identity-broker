import { lazy, Suspense, useCallback, useId, useLayoutEffect, useRef, useState } from 'react';
import { Check, ChevronDown, LockKeyhole, TriangleAlert } from 'lucide-react';
import { Checkbox } from '@design-system/components/inputs/Checkbox';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Button } from '@design-system/components/primitives/Button';
import { cn } from '@design-system/utils/cn';
import { consentCopy } from '@copy';
import { delegationsCopy } from '@copy/delegations';
import type { ServiceRequirement } from '@app-types/consent';
import type { ConsentDraft } from './consentDraft';
import { PermissionServiceList } from './PermissionServiceList';
import type { PermissionServiceListProps } from './PermissionServiceList';

const ServiceChoicesPopover = lazy(() => import('./ServiceChoicesPopover'));

export interface PermissionPanelProps {
  draft: ConsentDraft;
  services: ServiceRequirement[];
  disabled?: boolean;
  onChange: (draft: ConsentDraft) => void;
  mode?: 'decision' | 'console';
  onConnect?: (serviceId: string) => void;
  missingServiceIds?: readonly string[];
  error?: string;
  errorGroupId?: string;
}

export function PermissionPanel({ draft, services, disabled, onChange, mode = 'decision', onConnect, missingServiceIds = [], error, errorGroupId }: PermissionPanelProps) {
  const headingId = useId();
  const scrollRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const [overflow, setOverflow] = useState(false);
  const [remaining, setRemaining] = useState(0);
  const serviceNames = new Map(services.map(({ serviceId, serviceName }) => [serviceId, serviceName]));
  const missing = new Set(missingServiceIds);
  const groups = draft.groups;

  useLayoutEffect(() => {
    const scroll = scrollRef.current;
    const list = listRef.current;
    if (!scroll || !list) return;
    const measure = () => {
      setOverflow(scroll.scrollHeight > scroll.clientHeight);
      const bottom = scroll.getBoundingClientRect().bottom;
      setRemaining(Array.from(list.children).filter((row) => row.getBoundingClientRect().bottom > bottom + 1).length);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(scroll);
    observer.observe(list);
    scroll.addEventListener('scroll', measure);
    return () => { observer.disconnect(); scroll.removeEventListener('scroll', measure); };
  }, [draft]);

  return <section data-testid="permission-groups" className={cn(
    'relative flex min-h-0 flex-col overflow-hidden rounded-xl text-sm',
    mode === 'console' ? 'border border-border-subtle bg-card p-4' : 'bg-muted p-3',
  )}>
    <h2 id={headingId} className={cn('shrink-0 font-display font-semibold text-foreground', mode === 'console' ? 'mb-2 text-base' : 'mb-1 text-sm')}>{mode === 'console' ? delegationsCopy.permissions : consentCopy.ableTo}</h2>
    {error && !errorGroupId && <p role="alert" className="mb-1 text-xs text-status-danger-foreground">{error}</p>}
    <div ref={scrollRef} role="region" tabIndex={overflow ? 0 : undefined} aria-labelledby={headingId} className={cn('min-h-0 overflow-y-auto overscroll-contain focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring', overflow && '[mask-image:linear-gradient(to_bottom,#000_calc(100%-10px),transparent)]')}>
      {groups.length === 0 && <p className="py-2 text-muted-foreground">{consentCopy.noPermissions}</p>}
      <ul ref={listRef} className="divide-y divide-border-subtle">
        {groups.map((group) => {
          const nameId = `${headingId}-${group.id}-name`;
          const descriptionId = `${headingId}-${group.id}-description`;
          const checkboxId = `${headingId}-${group.id}-checkbox`;
          const Label = mode === 'console' && group.readOnly ? 'span' : 'label';
          const canChooseServices = group.services.length > 1 && (!group.readOnly || group.services.some((service) => !service.readOnly));
          const showConsoleServices = mode === 'console' && (group.services.length > 1 || group.services.some((service) => !service.required));
          const missingService = group.selected ? group.services.find((service) => service.selected && missing.has(service.id)) : undefined;
          const connectLabel = missingService ? consentCopy.connectService(serviceNames.get(missingService.id) ?? missingService.id) : undefined;
          const serviceListProps = { group, draft, serviceNames, missing, disabled, onChange, onConnect, mode };
          return <li key={group.id} data-testid="permission-group" data-selected={group.selected} data-read-only={group.readOnly} className="min-w-0 py-2">
            <div className="flex min-w-0 items-start gap-2">
              {mode === 'console' && group.readOnly
                ? <LockKeyhole aria-hidden="true" className="mt-0.5 size-5 shrink-0 p-0.5 text-muted-foreground" />
                : <Checkbox id={checkboxId} aria-labelledby={nameId} aria-describedby={descriptionId} checked={group.selected} disabled={disabled || group.readOnly} className={mode === 'console' ? 'size-5' : undefined} onCheckedChange={(checked) => onChange(draft.setPermissionSet(group.id, checked === true))} />}
              <div className="min-w-0 flex-1">
                <Label id={nameId} htmlFor={Label === 'label' ? checkboxId : undefined} data-testid="permission-group-name" className={cn('block font-medium text-foreground [overflow-wrap:anywhere]', Label === 'label' && 'cursor-pointer')}>{group.name}</Label>
                {group.required && <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">{mode === 'decision' && <LockKeyhole aria-hidden="true" className="size-3" />}{consentCopy.required}</span>}
                {mode === 'console' ? <Label id={descriptionId} htmlFor={Label === 'label' ? checkboxId : undefined} data-testid="permission-group-description" className="block text-xs text-muted-foreground [overflow-wrap:anywhere]">{group.description}</Label> : <TruncatedText id={descriptionId} data-testid="permission-group-description" as={Label} htmlFor={Label === 'label' ? checkboxId : undefined} text={group.description} lines={2} expandLabel={consentCopy.more} collapseLabel={consentCopy.less} className="text-xs text-muted-foreground [&>label]:cursor-pointer [&>button]:h-auto [&>button]:border-0 [&>button]:bg-transparent [&>button]:p-0 [&>button]:text-xs [&>button]:underline" />}
                {group.alreadyGranted && <span data-testid="permission-group-granted" className="inline-flex items-center gap-0.5 text-xs text-muted-foreground"><Check aria-hidden="true" className="size-3" />{consentCopy.granted}</span>}
                {mode === 'console' && group.services.length === 1 && !showConsoleServices && <p data-testid="permission-service-name" className="mt-1 text-xs text-muted-foreground [overflow-wrap:anywhere]">{serviceNames.get(group.services[0]!.id) ?? group.services[0]!.id}</p>}
              </div>
              <div className="flex shrink-0 flex-col items-end gap-1 pt-0.5">
                <div className="flex items-center gap-1">
                  {mode === 'decision' && missingService && onConnect && <Button type="button" size="sm" variant="outline" disabled={disabled} aria-label={connectLabel} onClick={() => onConnect(missingService.id)} className="px-2">{consentCopy.connect}</Button>}
                  {mode === 'decision' ? <div role="group" className="flex items-center gap-1" aria-label={consentCopy.services}>
                    {group.services.map((service) => {
                      const name = serviceNames.get(service.id) ?? service.id;
                      return <span key={service.id} className={cn('inline-flex items-center gap-1', !service.selected && 'opacity-50')}>
                        <Avatar id={service.id} label={name} className="size-6 text-xs" />
                        {service.selected && missing.has(service.id) && <TriangleAlert aria-label={consentCopy.missingConnection(name)} className="size-3 shrink-0 text-warning-foreground" />}
                      </span>;
                    })}
                  </div> : null}
                </div>
                {mode === 'decision' && canChooseServices && <ServiceChoicesTrigger {...serviceListProps} />}
              </div>
            </div>
            {showConsoleServices && <div className="mt-2 pl-7"><PermissionServiceList {...serviceListProps} /></div>}
            {error && errorGroupId === group.id && <p role="alert" className="mt-1 pl-7 text-xs text-status-danger-foreground">{error}</p>}
          </li>;
        })}
      </ul>
    </div>
    {overflow && <p className="h-4 shrink-0 pt-0.5 text-center text-xs text-muted-foreground" data-testid="permission-overflow-hint">{remaining > 0 && consentCopy.morePermissions(remaining)}</p>}
  </section>;
}

function ServiceChoicesTrigger(listProps: PermissionServiceListProps) {
  const [open, setOpen] = useState(false);
  const [ready, setReady] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const onReady = useCallback(() => setReady(true), []);

  return <>
    <button ref={trigger} type="button" className="inline-flex items-center gap-1 rounded-sm text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
      aria-label={consentCopy.chooseServices(listProps.group.name)} aria-haspopup="dialog" aria-expanded={open} aria-busy={open && !ready || undefined}
      onClick={() => setOpen((before) => !before)}
      onKeyDown={(event) => { if (event.key === 'Escape' && !ready) setOpen(false); }}>{consentCopy.selectedServices(listProps.group.services.filter((service) => service.selected).length, listProps.group.services.length)}<ChevronDown aria-hidden="true" className="size-3.5" /></button>
    {(open || ready) && <Suspense fallback={null}>
      <ServiceChoicesPopover {...listProps} trigger={trigger} open={open} onOpenChange={setOpen} onReady={onReady} />
    </Suspense>}
  </>;
}
