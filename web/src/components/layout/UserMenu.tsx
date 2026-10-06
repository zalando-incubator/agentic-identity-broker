import { Link } from 'react-router-dom';
import { ArrowUpRight, ChevronDown, Settings } from 'lucide-react';
import { Avatar } from '@design-system/components/primitives/Avatar/Avatar';
import { Button } from '@design-system/components/primitives/Button/Button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@design-system/components/overlays/DropdownMenu/DropdownMenu';
import { ThemeChoice } from '@design-system/theme/ThemeChoice';
import { usePrincipal } from '@services/query/QueryProvider';
import { navigationCopy, themeCopy } from '@copy';

export default function UserMenu() {
  const user = usePrincipal();
  const displayName = user.displayName || user.principal;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" className="w-full min-w-0 justify-start px-2 group-data-[state=collapsed]/sidebar:justify-center" aria-label={navigationCopy.userMenu}>
          <Avatar person label={displayName} src={user.pictureUrl} size="sm" />
          <span className="min-w-0 flex-1 truncate text-left group-data-[state=collapsed]/sidebar:hidden" title={displayName}>{displayName}</span>
          <ChevronDown aria-hidden="true" className="group-data-[state=collapsed]/sidebar:hidden" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuLabel className="space-y-1 [overflow-wrap:anywhere]">
          <p className="text-sm font-medium">{displayName}</p>
          {user.principal !== displayName && (
            <p className="text-xs font-normal text-muted-foreground">{user.principal}</p>
          )}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild><Link to="/settings/appearance"><Settings aria-hidden="true" className="size-4" />{navigationCopy.settings}</Link></DropdownMenuItem>
        <DropdownMenuLabel>{themeCopy.label}</DropdownMenuLabel>
        <ThemeChoice presentation="menu" labels={themeCopy} aria-label={themeCopy.label} />
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild><a href={navigationCopy.documentationUrl} target="_blank" rel="noopener noreferrer">{navigationCopy.documentation}<ArrowUpRight aria-hidden="true" className="ml-auto size-4" /></a></DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
