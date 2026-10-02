import { useRef } from 'react';
import { accessCopy, commonCopy } from '@copy';
import { Button } from '@design-system/components/primitives/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@design-system/components/overlays/Dialog';

export interface RevokeAgentDialogProps {
  open: boolean;
  agentName: string;
  pending?: boolean;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
  /** Used when an overflow-menu item unmounts before this controlled dialog opens. */
  onReturnFocus?: () => void;
}

export function RevokeAgentDialog({
  open,
  agentName,
  pending = false,
  onCancel,
  onConfirm,
  onReturnFocus,
}: RevokeAgentDialogProps) {
  const cancelButton = useRef<HTMLButtonElement>(null);
  const opener = useRef<HTMLElement | null>(null);

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => { if (!nextOpen) onCancel(); }}>
      <DialogContent
        closeLabel={commonCopy.close}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
          cancelButton.current?.focus();
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          if (onReturnFocus) onReturnFocus();
          else if (opener.current?.isConnected) opener.current.focus();
        }}
      >
        <DialogHeader>
          <DialogTitle>{accessCopy.revokeTitle}</DialogTitle>
          <DialogDescription className="break-words">{accessCopy.revokeDescription(agentName)}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button ref={cancelButton} variant="secondary" onClick={onCancel}>{commonCopy.cancel}</Button>
          <Button variant="destructive" isLoading={pending} disabled={pending} onClick={onConfirm}>
            {accessCopy.revoke}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
