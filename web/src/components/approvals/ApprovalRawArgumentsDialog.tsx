import { Button } from '@design-system/components/primitives/Button';
import { Braces } from 'lucide-react';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@design-system/components/overlays/Dialog';
import { commonCopy } from '@copy';
import { approvalQueueCopy } from '@copy/approvalQueue';
import type { ToolApprovalDetail } from '../../types/approval';

interface ApprovalRawArgumentsDialogProps {
  arguments: ToolApprovalDetail['arguments'];
}

export default function ApprovalRawArgumentsDialog({ arguments: toolArguments }: ApprovalRawArgumentsDialogProps) {
  return <Dialog defaultOpen>
    <DialogTrigger asChild><Button variant="ghost" size="sm" className="h-auto min-h-8 px-1 text-xs text-muted-foreground hover:text-foreground"><Braces aria-hidden="true" />{approvalQueueCopy.viewJson}</Button></DialogTrigger>
    <DialogContent closeLabel={commonCopy.close}>
      <DialogHeader><DialogTitle>{approvalQueueCopy.rawArguments}</DialogTitle><DialogDescription>{approvalQueueCopy.rawArgumentsDescription}</DialogDescription></DialogHeader>
      <pre data-testid="approval-arguments" className="max-h-[60dvh] overflow-auto whitespace-pre-wrap rounded-md border border-border-subtle bg-muted p-3 font-mono text-xs [overflow-wrap:anywhere]">{JSON.stringify(toolArguments, null, 2)}</pre>
    </DialogContent>
  </Dialog>;
}
