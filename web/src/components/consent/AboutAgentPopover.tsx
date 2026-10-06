import { useEffect, useRef } from 'react';
import type { RefObject } from 'react';
import { ExternalLink } from 'lucide-react';
import { Popover, PopoverAnchor, PopoverContent } from '@design-system/components/overlays/Popover';
import { consentCopy } from '@copy/consent';

interface AboutAgentPopoverProps {
  description?: string;
  links: { href: string; label: string }[];
  trigger: RefObject<HTMLButtonElement | null>;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onReady: () => void;
}

export default function AboutAgentPopover({ description, links, trigger, open, onOpenChange, onReady }: AboutAgentPopoverProps) {
  const outside = useRef(false);
  useEffect(() => { onReady(); }, [onReady]);

  return <Popover open={open} onOpenChange={onOpenChange}>
    <PopoverAnchor virtualRef={trigger} />
    <PopoverContent align="end" className="space-y-3" aria-label={consentCopy.about}
      onInteractOutside={(event) => {
        if (event.target instanceof Node && trigger.current?.contains(event.target)) event.preventDefault();
        else outside.current = true;
      }}
      onCloseAutoFocus={(event) => {
        event.preventDefault();
        if (!outside.current) trigger.current?.focus();
        outside.current = false;
      }}>
      <p className="font-display text-sm font-semibold">{consentCopy.about}</p>
      {description && <p className="max-h-40 overflow-y-auto text-sm [overflow-wrap:anywhere]">{description}</p>}
      {links.length > 0 && <ul className="space-y-2">{links.map(({ href, label }) => <li key={label}>
        <a href={href} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 text-sm text-primary underline-offset-4 hover:underline">{label}<ExternalLink aria-hidden="true" className="size-3" /></a>
      </li>)}</ul>}
    </PopoverContent>
  </Popover>;
}
