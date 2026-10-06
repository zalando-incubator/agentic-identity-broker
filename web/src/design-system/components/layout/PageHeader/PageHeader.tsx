import type { ReactNode } from 'react';

export interface PageHeaderProps {
  title: string;
  purpose?: string;
  count?: number;
  actions?: ReactNode;
}

export function PageHeader({ title, purpose, count, actions }: PageHeaderProps) {
  return (
    <div className="mb-6 flex min-w-0 flex-wrap items-start justify-between gap-x-6 gap-y-3">
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-baseline gap-2">
          <h1 className="min-w-0 break-words font-display text-xl font-semibold leading-7 text-foreground">{title}</h1>
          {count !== undefined && <span className="shrink-0 font-sans text-sm font-normal tabular-nums text-muted-foreground">· {count}</span>}
        </div>
        {purpose && <p className="mt-1 text-sm leading-6 text-muted-foreground">{purpose}</p>}
      </div>
      {actions && <div className="flex max-w-full flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}
