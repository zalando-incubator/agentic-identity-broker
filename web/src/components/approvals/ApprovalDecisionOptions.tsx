import type { Ref } from 'react';
import { ChevronDown } from 'lucide-react';
import { Button } from '@design-system/components/primitives/Button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@design-system/components/overlays/DropdownMenu';
import { approvalCopy } from '@copy/approvals';
import { approvalQueueCopy } from '@copy/approvalQueue';

type ApprovalDecisionOptionsProps = {
  kind: 'deny';
  disabled: boolean;
  triggerRef: Ref<HTMLButtonElement>;
  onSelect: () => void;
} | {
  kind: 'approve';
  disabled: boolean;
  onSelect: (persistence: 'session' | 'permanent') => void;
};

export default function ApprovalDecisionOptions(props: ApprovalDecisionOptionsProps) {
  return <DropdownMenu defaultOpen>
    <DropdownMenuTrigger asChild>
      <Button ref={props.kind === 'deny' ? props.triggerRef : undefined} variant={props.kind === 'deny' ? 'outline' : 'primary'} size="icon" aria-label={props.kind === 'deny' ? approvalQueueCopy.denyOptions : approvalQueueCopy.approveOptions} disabled={props.disabled} className={props.kind === 'deny' ? 'rounded-l-none border-0 border-l border-border' : 'rounded-l-none border-0 border-l border-primary-foreground/30'}><ChevronDown aria-hidden="true" /></Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align={props.kind === 'deny' ? 'start' : 'end'} onFocusCapture={event => {
      if (event.target === event.currentTarget) event.currentTarget.querySelector<HTMLElement>('[role="menuitem"]:not([data-disabled])')?.focus({ preventScroll: true });
    }}>
      {props.kind === 'deny'
        ? <DropdownMenuItem disabled={props.disabled} onSelect={props.onSelect}>{approvalQueueCopy.denyPermanently}</DropdownMenuItem>
        : <>
          <DropdownMenuItem disabled={props.disabled} onSelect={() => props.onSelect('session')}>{approvalCopy.session}</DropdownMenuItem>
          <DropdownMenuItem disabled={props.disabled} onSelect={() => props.onSelect('permanent')}>{approvalQueueCopy.always}</DropdownMenuItem>
        </>}
    </DropdownMenuContent>
  </DropdownMenu>;
}
