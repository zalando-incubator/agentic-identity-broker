import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { cn } from '@design-system/utils/cn';

export interface SettingsCategory {
  href: string;
  label: string;
}

export interface SettingsLayoutProps {
  categories: readonly SettingsCategory[];
  currentHref: string;
  navigationLabel: string;
  children: ReactNode;
}

export function SettingsLayout({ categories, currentHref, navigationLabel, children }: SettingsLayoutProps) {
  const showCategories = categories.length > 1;
  return (
    <div className="console-grid">
      {showCategories && (
        <nav aria-label={navigationLabel} className="col-span-12 flex min-w-0 flex-wrap gap-1 min-[1012px]:col-span-3 min-[1012px]:flex-col">
          {categories.map(category => (
            <Link key={category.href} to={category.href} aria-current={category.href === currentHref ? 'page' : undefined}
              className={cn('block min-w-0 rounded-lg px-3 py-2 text-sm font-medium text-foreground outline-none transition-colors duration-(--motion-feedback) ease-(--motion-ease) hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
                category.href === currentHref && 'bg-primary-soft text-primary-soft-foreground hover:bg-primary-soft')}>
              {category.label}
            </Link>
          ))}
        </nav>
      )}
      <div className={cn('col-span-12 min-w-0', showCategories && 'min-[1012px]:col-span-9')}>
        {children}
      </div>
    </div>
  );
}
