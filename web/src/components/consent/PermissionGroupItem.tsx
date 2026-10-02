import { useId } from 'react';
import { FolderKey, LockKeyhole } from 'lucide-react';
import { AccordionItem, AccordionTrigger, AccordionContent } from '@design-system/components/advanced/Accordion';
import { Checkbox } from '@design-system/components/inputs/Checkbox';
import { Badge } from '@design-system/components/primitives/Badge';
import { consentCopy } from '@copy/consent';
import type { ServiceRequirement } from '../../types/consent';
import type { DraftPermissionGroup } from './consentDraft';

export interface PermissionGroupItemProps {
  group: DraftPermissionGroup;
  services: ServiceRequirement[];
  expanded: boolean;
  disabled?: boolean;
  onSelectionChange: (selected: boolean) => void;
  onServiceChange: (serviceId: string, selected: boolean) => void;
}

export function PermissionGroupItem({ group, services, expanded, disabled, onSelectionChange, onServiceChange }: PermissionGroupItemProps) {
  const descriptionId = useId();
  return <AccordionItem value={group.id} data-testid="permission-group" className="py-3">
    <div className="flex items-start gap-3">
      <Checkbox className="mt-3" aria-label={group.name} aria-describedby={descriptionId} checked={group.selected} disabled={disabled || group.readOnly} onCheckedChange={(value) => onSelectionChange(value === true)} />
      <div className="min-w-0 flex-1">
        <AccordionTrigger className="px-0">
          <span className="flex min-w-0 items-start gap-2"><FolderKey aria-hidden="true" className="size-4 shrink-0" /><span data-testid="permission-group-name" className={`[overflow-wrap:anywhere] ${expanded ? '' : 'line-clamp-2'}`}>{group.name}</span></span>
        </AccordionTrigger>
        <p id={descriptionId} data-testid="permission-group-description" className={`text-sm text-muted-foreground [overflow-wrap:anywhere] ${expanded ? '' : 'line-clamp-2'}`}>{group.description}</p>
        <div className="mt-2 flex flex-wrap gap-2">
          {group.required ? <Badge variant="outline" data-testid="permission-group-required"><LockKeyhole aria-hidden="true" className="size-3" />{consentCopy.required}</Badge> : <Badge variant="neutral">{consentCopy.optional}</Badge>}
          {group.alreadyGranted && <Badge variant="neutral" data-testid="permission-group-granted">{consentCopy.alreadyGranted}</Badge>}
        </div>
      </div>
    </div>
    <AccordionContent className="mt-3 pl-9">
      <ul className="space-y-3" aria-label={consentCopy.services}>
        {group.services.map((service) => {
          const name = services.find((entry) => entry.serviceId === service.id)?.serviceName ?? service.id;
          return <li key={service.id} className="flex items-center justify-between gap-3" data-testid="permission-service">
            <Checkbox checked={service.selected} disabled={disabled || service.readOnly} onCheckedChange={(value) => onServiceChange(service.id, value === true)} label={<span data-testid="permission-service-name" className="[overflow-wrap:anywhere]">{name}</span>} />
            {group.selected && service.required && <Badge variant="outline">{consentCopy.required}</Badge>}
          </li>;
        })}
      </ul>
    </AccordionContent>
  </AccordionItem>;
}
