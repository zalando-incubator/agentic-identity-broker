import type { ReactElement } from 'react';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@design-system/components/overlays/Tooltip';

export default function TruncatedTextTooltip({ children, text }: { children: ReactElement; text: string }) {
  return <TooltipProvider><Tooltip><TooltipTrigger asChild>{children}</TooltipTrigger><TooltipContent>{text}</TooltipContent></Tooltip></TooltipProvider>;
}
