import type { ReactNode } from 'react';

export interface PageHeaderProps {
  title: string;
  purpose: string;
  action?: ReactNode;
}

export function PageHeader({ title, purpose, action }: PageHeaderProps) {
  return (
    <div className="mb-6 flex min-w-0 flex-wrap items-start justify-between gap-4">
      <div className="min-w-0 flex-1">
        <h1 className="break-words font-display text-2xl font-semibold text-foreground">{title}</h1>
        <p className="mt-1 text-sm leading-6 text-muted-foreground">{purpose}</p>
      </div>
      {action && <div className="flex max-w-full flex-wrap items-center gap-2">{action}</div>}
    </div>
  );
}
