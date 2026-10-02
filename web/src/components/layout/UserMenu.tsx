import { Avatar } from '@design-system/components/primitives/Avatar/Avatar';
import { Button } from '@design-system/components/primitives/Button/Button';
import {
  DropdownMenu,
  DropdownMenuContent,
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
        <Button variant="ghost" size="icon" aria-label={navigationCopy.userMenu}>
          <Avatar label={displayName} src={user.pictureUrl} size="sm" />
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
        <DropdownMenuLabel>{themeCopy.label}</DropdownMenuLabel>
        <ThemeChoice presentation="menu" labels={themeCopy} aria-label={themeCopy.label} />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
