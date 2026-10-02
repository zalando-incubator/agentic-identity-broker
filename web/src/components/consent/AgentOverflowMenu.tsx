import { useRef } from 'react';
import { MoreHorizontal } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { Button } from '@design-system/components/primitives/Button';
import { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem } from '@design-system/components/overlays/DropdownMenu';
import { Alert } from '@design-system/components/feedback/Alert';
import { toast } from '@design-system/components/feedback/Toaster';
import { accessCopy } from '@copy';
import { consentCopy } from '@copy/consent';
import { useRevokeGrant, type RevokeAgentTarget } from '@hooks/useRevokeGrant';
import { RevokeAgentDialog } from './RevokeAgentDialog';
import { consentErrorMessage } from './consentValidation';

export function AgentOverflowMenu({ agent, disabled }: { agent: RevokeAgentTarget; disabled?: boolean }) {
  const trigger = useRef<HTMLButtonElement>(null);
  const navigate = useNavigate();
  const revoke = useRevokeGrant({ onSuccess: () => { toast.success(consentCopy.grantRevoked); navigate('/delegations'); } });
  const pending = revoke.isPending(agent.agentId);
  return <div>
    <DropdownMenu>
      <DropdownMenuTrigger asChild><Button ref={trigger} variant="ghost" size="icon" aria-label={consentCopy.actions} disabled={pending || disabled}><MoreHorizontal aria-hidden="true" className="size-5" /></Button></DropdownMenuTrigger>
      <DropdownMenuContent align="end" onCloseAutoFocus={(event) => { if (revoke.confirmation) event.preventDefault(); }}>
        <DropdownMenuItem onSelect={() => revoke.requestRevoke(agent)}>{accessCopy.revokeAllAccess}</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
    <RevokeAgentDialog open={revoke.confirmation !== null} agentName={agent.displayName} pending={pending} onCancel={revoke.cancelRevoke} onConfirm={revoke.confirmRevoke} onReturnFocus={() => trigger.current?.focus()} />
    {Boolean(revoke.error) && <Alert variant="error">{consentErrorMessage(revoke.error, consentCopy.grantRevokeError)}</Alert>}
  </div>;
}
