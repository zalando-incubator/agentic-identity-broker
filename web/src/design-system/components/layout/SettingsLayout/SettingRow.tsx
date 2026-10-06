import type { ReactNode } from 'react';

export interface SettingRowProps {
  label: string;
  description: string;
  control: ReactNode;
}

export function SettingRow({ label, description, control }: SettingRowProps) {
  return (
    <div className="flex min-w-0 flex-col gap-3 border-b border-border-subtle py-6 first:pt-0 last:border-b-0 min-[768px]:flex-row min-[768px]:items-center min-[768px]:gap-6">
      <div className="min-w-0 min-[768px]:w-64 min-[768px]:shrink-0">
        <h3 className="font-sans text-sm font-medium text-foreground">{label}</h3>
        <p className="mt-1 text-sm text-muted-foreground">{description}</p>
      </div>
      <div className="w-full min-w-0 min-[768px]:w-auto min-[768px]:shrink-0">{control}</div>
    </div>
  );
}
