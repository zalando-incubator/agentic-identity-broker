import { useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { commonCopy, navigationCopy, themeCopy } from '@copy';
import { commandCopy } from '@copy/command';
import { useDelegations } from '@hooks/useDelegations';
import { useConnections } from '@hooks/useConnections';
import { usePendingApprovals } from '@hooks/usePendingApprovals';
import { useTheme } from '@design-system/theme/ThemeProvider';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@design-system/components/overlays/Dialog';
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandLoading } from '@design-system/components/advanced/Command';

export interface CommandPaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onNavigate?: () => void;
}

export function CommandPalette({ open, onOpenChange, onNavigate }: CommandPaletteProps) {
  const navigate = useNavigate();
  const agents = useDelegations();
  const connections = useConnections();
  const approvals = usePendingApprovals();
  const { setTheme } = useTheme();
  const input = useRef<HTMLInputElement>(null);
  const opener = useRef<HTMLElement | null>(null);
  const navigateTo = (path: string) => {
    onNavigate?.();
    onOpenChange(false);
    navigate(path);
  };
  const loading = agents.loading || connections.loading || approvals.isPending;
  const failed = Boolean(agents.error || connections.error || approvals.error);

  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent closeLabel={commonCopy.close} className="gap-3 p-4" onOpenAutoFocus={(event) => {
      event.preventDefault();
      opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      input.current?.focus();
    }} onCloseAutoFocus={(event) => {
      event.preventDefault();
      if (opener.current?.isConnected) opener.current.focus();
    }}>
      <DialogHeader>
        <DialogTitle>{commandCopy.title}</DialogTitle>
        <DialogDescription>{commandCopy.description}</DialogDescription>
      </DialogHeader>
      <Command label={commandCopy.search} loop>
        <CommandInput ref={input} placeholder={commandCopy.search} />
        <CommandList label={commandCopy.results}>
          <CommandGroup heading={navigationCopy.agents}>
            {agents.delegations.map((agent) => <CommandItem key={agent.agentId} value={`agent:${agent.agentId}`} keywords={[agent.displayName]} onSelect={() => navigateTo(`/agents/${encodeURIComponent(agent.agentId)}`)}>{agent.displayName}</CommandItem>)}
          </CommandGroup>
          <CommandGroup heading={navigationCopy.connections}>
            {connections.sessions.map((session) => <CommandItem key={session.id} value={`connection:${session.id}`} keywords={[session.service_display_name]} onSelect={() => navigateTo('/sessions')}>{session.service_display_name}</CommandItem>)}
          </CommandGroup>
          <CommandGroup heading={navigationCopy.approvals}>
            {(approvals.data ?? []).map((approval) => <CommandItem key={approval.id} value={`approval:${approval.id}`} keywords={[approval.tool_name, approval.agent_id]} onSelect={() => navigateTo(`/approvals/${encodeURIComponent(approval.id)}`)}>{approval.tool_name}</CommandItem>)}
          </CommandGroup>
          <CommandGroup heading={commandCopy.theme}>
            {(['light', 'dark', 'system'] as const).map((preference) => <CommandItem key={preference} value={`theme:${preference}`} keywords={[themeCopy[preference], commandCopy.theme]} onSelect={() => { setTheme(preference); onOpenChange(false); }}>{themeCopy[preference]}</CommandItem>)}
          </CommandGroup>
        </CommandList>
        {loading ? <CommandLoading label={commonCopy.loading}>{commonCopy.loading}</CommandLoading> : <CommandEmpty>{commandCopy.noResults}</CommandEmpty>}
      </Command>
      {failed ? <p role="status" className="text-sm text-muted-foreground">{commandCopy.loadFailure}</p> : approvals.stale && <p role="status" className="text-sm text-muted-foreground">{commandCopy.stale}</p>}
    </DialogContent>
  </Dialog>;
}

export default CommandPalette;
