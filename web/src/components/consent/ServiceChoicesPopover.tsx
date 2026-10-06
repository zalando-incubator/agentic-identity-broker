import { useEffect, useRef } from 'react';
import type { RefObject } from 'react';
import { Popover, PopoverAnchor, PopoverContent } from '@design-system/components/overlays/Popover';
import { consentCopy } from '@copy';
import { PermissionServiceList } from './PermissionServiceList';
import type { PermissionServiceListProps } from './PermissionServiceList';

interface ServiceChoicesPopoverProps extends PermissionServiceListProps {
  trigger: RefObject<HTMLButtonElement | null>;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onReady: () => void;
}

export default function ServiceChoicesPopover({ trigger, open, onOpenChange, onReady, ...listProps }: ServiceChoicesPopoverProps) {
  const outside = useRef(false);
  useEffect(() => { onReady(); }, [onReady]);

  return <Popover open={open} onOpenChange={onOpenChange}>
    <PopoverAnchor virtualRef={trigger} />
    <PopoverContent align="start" data-testid="permission-services-popover" aria-label={consentCopy.servicesFor(listProps.group.name)} className="w-72 space-y-2"
      onInteractOutside={(event) => {
        if (event.target instanceof Node && trigger.current?.contains(event.target)) event.preventDefault();
        else outside.current = true;
      }}
      onCloseAutoFocus={(event) => {
        event.preventDefault();
        if (!outside.current) trigger.current?.focus();
        outside.current = false;
      }}>
      <p className="font-medium">{consentCopy.servicesFor(listProps.group.name)}</p>
      <PermissionServiceList {...listProps} />
    </PopoverContent>
  </Popover>;
}
