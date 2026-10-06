import { Check, ChevronDown } from 'lucide-react';
import { Button } from '@design-system/components/primitives/Button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@design-system/components/overlays/DropdownMenu';
import { accessCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';
import { approvalQueueCopy } from '@copy/approvalQueue';

type ApprovalDecisionOptionsProps = {
  kind: 'deny';
  disabled: boolean;
  selected: boolean;
  onSelect: (permanent: boolean) => void;
} | {
  kind: 'approve';
  disabled: boolean;
  selected: 'once' | 'session' | 'permanent';
  onSelect: (persistence: 'once' | 'session' | 'permanent') => void;
};

export default function ApprovalDecisionOptions(props: ApprovalDecisionOptionsProps) {
  return <DropdownMenu defaultOpen>
    <DropdownMenuTrigger asChild>
      <Button variant={props.kind === 'deny' ? 'outline' : 'primary'} size="icon" aria-label={props.kind === 'deny' ? approvalQueueCopy.denyOptions : approvalQueueCopy.approveOptions} disabled={props.disabled} className={props.kind === 'deny' ? 'rounded-l-none border-0 border-l border-border' : 'rounded-l-none border-0 border-l border-primary-foreground/30'}><ChevronDown aria-hidden="true" /></Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align={props.kind === 'deny' ? 'start' : 'end'} onFocusCapture={event => {
      if (event.target === event.currentTarget) event.currentTarget.querySelector<HTMLElement>('[role="menuitem"]:not([data-disabled])')?.focus({ preventScroll: true });
    }}>
      {props.kind === 'deny'
        ? <>
          <DropdownMenuItem disabled={props.disabled} onSelect={() => props.onSelect(false)}>{!props.selected && <Check aria-hidden="true" />}{accessCopy.deny}</DropdownMenuItem>
          <DropdownMenuItem disabled={props.disabled} onSelect={() => props.onSelect(true)}>{props.selected && <Check aria-hidden="true" />}{approvalQueueCopy.denyPermanently}</DropdownMenuItem>
        </>
        : <>
          <DropdownMenuItem disabled={props.disabled} onSelect={() => props.onSelect('once')}>{props.selected === 'once' && <Check aria-hidden="true" />}{accessCopy.approveOnce}</DropdownMenuItem>
          <DropdownMenuItem disabled={props.disabled} onSelect={() => props.onSelect('session')}>{props.selected === 'session' && <Check aria-hidden="true" />}{approvalCopy.session}</DropdownMenuItem>
          <DropdownMenuItem disabled={props.disabled} onSelect={() => props.onSelect('permanent')}>{props.selected === 'permanent' && <Check aria-hidden="true" />}{approvalQueueCopy.always}</DropdownMenuItem>
        </>}
    </DropdownMenuContent>
  </DropdownMenu>;
}
