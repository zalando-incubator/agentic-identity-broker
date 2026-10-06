import { useId, useRef, type ReactNode } from 'react';
import { Wordmark } from '@design-system/components/primitives/Wordmark/Wordmark';

export interface DecisionShellProps {
  children: ReactNode;
  wordmarkLabel: string;
  skipToMainLabel: string;
  compact?: boolean;
}

export function DecisionShell({ children, wordmarkLabel, skipToMainLabel, compact = false }: DecisionShellProps) {
  const mainId = useId();
  const mainRef = useRef<HTMLElement>(null);
  const width = compact ? 'max-w-[512px]' : 'max-w-[640px]';

  return (
    <div data-slot="decision-shell" className="flex min-h-dvh min-w-0 flex-col bg-background text-foreground">
      <a
        href={`#${mainId}`}
        onClick={(event) => {
          event.preventDefault();
          mainRef.current?.focus();
        }}
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-100 focus:rounded-md focus:bg-background focus:px-4 focus:py-2 focus:text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
      >
        {skipToMainLabel}
      </a>
      <div className={`mx-auto flex w-full min-w-0 flex-1 flex-col justify-center px-4 py-4 max-sm:justify-start max-sm:px-0 max-sm:py-0 ${width}`}>
        <header className="flex w-full shrink-0 justify-center pb-4 max-sm:py-4">
          <Wordmark label={wordmarkLabel} className="h-7 max-sm:h-6" />
        </header>
        <main
          id={mainId}
          ref={mainRef}
          tabIndex={-1}
          className="flex w-full min-w-0 flex-col focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
        >
          {children}
        </main>
      </div>
    </div>
  );
}
