import { useId, useRef, type ReactNode } from 'react';
import { Wordmark } from '@design-system/components/primitives/Wordmark/Wordmark';

export interface DecisionShellProps {
  children: ReactNode;
  wordmarkLabel: string;
  skipToMainLabel: string;
  footerLabel?: string;
}

export function DecisionShell({ children, wordmarkLabel, skipToMainLabel, footerLabel }: DecisionShellProps) {
  const mainId = useId();
  const mainRef = useRef<HTMLElement>(null);

  return (
    <div className="flex min-h-dvh min-w-0 flex-col bg-background text-foreground">
      <a
        href={`#${mainId}`}
        onClick={(event) => {
          event.preventDefault();
          mainRef.current?.focus();
        }}
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-background focus:px-4 focus:py-2 focus:text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
      >
        {skipToMainLabel}
      </a>
      <header className="mx-auto w-full max-w-[640px] px-4 py-8">
        <Wordmark label={wordmarkLabel} />
      </header>
      <main
        id={mainId}
        ref={mainRef}
        tabIndex={-1}
        className="mx-auto w-full min-w-0 max-w-[640px] flex-1 px-4 pb-8 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
      >
        {children}
      </main>
      {footerLabel && (
        <footer className="mx-auto w-full max-w-[640px] px-4 py-6 text-center text-xs text-muted-foreground">
          {footerLabel}
        </footer>
      )}
    </div>
  );
}
