import { useState } from 'react';
import { Accordion } from '@design-system/components/advanced/Accordion';
import { consentCopy } from '@copy/consent';
import type { ServiceRequirement } from '../../types/consent';
import type { ConsentDraft } from './consentDraft';
import { PermissionGroupItem } from './PermissionGroupItem';

export function PermissionGroupList({ draft, services, disabled, onChange }: { draft: ConsentDraft; services: ServiceRequirement[]; disabled?: boolean; onChange: (draft: ConsentDraft) => void }) {
  const [expanded, setExpanded] = useState<string[]>([]);
  return <section aria-labelledby="permissions-heading">
    <h2 id="permissions-heading" className="font-display text-lg font-semibold">{consentCopy.permissions}</h2>
    <Accordion type="multiple" value={expanded} onValueChange={setExpanded} data-testid="permission-groups">
      {draft.groups.map((group) => <PermissionGroupItem key={group.id} group={group} services={services} expanded={expanded.includes(group.id)} disabled={disabled}
        onSelectionChange={(selected) => onChange(draft.setPermissionSet(group.id, selected))}
        onServiceChange={(serviceId, selected) => onChange(draft.setService(group.id, serviceId, selected))} />)}
    </Accordion>
    {draft.groups.length === 0 && <p className="text-sm text-muted-foreground">{consentCopy.noPermissions}</p>}
  </section>;
}
