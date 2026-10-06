import { useId } from 'react';
import { Check, LockKeyhole, TriangleAlert } from 'lucide-react';
import { Checkbox } from '@design-system/components/inputs/Checkbox';
import { Button } from '@design-system/components/primitives/Button';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { consentCopy } from '@copy';
import { cn } from '@design-system/utils/cn';
import type { ConsentDraft, DraftPermissionGroup } from './consentDraft';

export interface PermissionServiceListProps {
  group: DraftPermissionGroup;
  draft: ConsentDraft;
  serviceNames: Map<string, string>;
  missing: Set<string>;
  disabled?: boolean;
  mode?: 'decision' | 'console';
  onChange: (draft: ConsentDraft) => void;
  onConnect?: (serviceId: string) => void;
}

export function PermissionServiceList({ group, draft, serviceNames, missing, disabled, onChange, onConnect, mode = 'decision' }: PermissionServiceListProps) {
  const minimumId = useId();
  const removalBlocked = group.services.some((service) => service.removalBlocked && !service.readOnly);
  return <><ul className={mode === 'console' ? 'space-y-1' : 'space-y-2'} aria-label={consentCopy.servicesFor(group.name)}>
    {group.services.map((service) => {
      const name = serviceNames.get(service.id) ?? service.id;
      const selectionDisabled = disabled || service.readOnly || service.removalBlocked;
      const minimumDescription = service.removalBlocked && !service.readOnly ? minimumId : undefined;
      return <li key={service.id} data-testid="permission-service" data-required={service.required} data-selected={service.selected} data-read-only={service.readOnly} className="flex min-w-0 items-center gap-1">
        <div className="min-w-0 flex-1">{mode === 'console'
          ? service.required
            ? <span className="inline-flex items-start gap-2 py-1 text-xs text-muted-foreground"><LockKeyhole aria-hidden="true" className="mt-0.5 size-3 shrink-0" /><span><span data-testid="permission-service-name" className="[overflow-wrap:anywhere]">{name}</span> — {consentCopy.required}</span></span>
            : <Button type="button" variant="outline" size="sm" aria-pressed={service.selected} aria-describedby={minimumDescription} disabled={selectionDisabled} onClick={() => onChange(draft.setService(group.id, service.id, !service.selected))} className={cn('h-auto max-w-full justify-start gap-2 whitespace-normal py-1 text-xs', service.selected && 'bg-primary-soft text-primary-soft-foreground')}><Check aria-hidden="true" className={cn('size-3 shrink-0', !service.selected && 'invisible')} /><span data-testid="permission-service-name" className="[overflow-wrap:anywhere]">{name}</span></Button>
          : <Checkbox label={<span className="inline-flex items-center gap-2"><Avatar id={service.id} label={name} size="sm" aria-hidden="true" className="size-6 text-xs" /><span data-testid="permission-service-name" className="[overflow-wrap:anywhere]">{name}</span></span>} checked={service.selected} aria-describedby={minimumDescription} disabled={selectionDisabled} onCheckedChange={(checked) => onChange(draft.setService(group.id, service.id, checked === true))} />}</div>
        {mode === 'decision' && group.selected && service.required && <span className="text-xs text-muted-foreground">{consentCopy.required}</span>}
        {mode === 'decision' && service.selected && missing.has(service.id) && <TriangleAlert aria-label={consentCopy.missingConnection(name)} className="size-4 shrink-0 text-warning-foreground" />}
        {mode === 'decision' && service.selected && missing.has(service.id) && onConnect && <Button type="button" size="sm" variant="outline" disabled={disabled} aria-label={consentCopy.connectService(name)} title={consentCopy.connectService(name)} onClick={() => onConnect(service.id)} className="max-w-28 px-2"><span className="min-w-0 truncate">{consentCopy.connectService(name)}</span></Button>}
      </li>;
    })}
  </ul>{removalBlocked && <p id={minimumId} className="mt-1 text-xs text-muted-foreground">{consentCopy.keepService}</p>}</>;
}
