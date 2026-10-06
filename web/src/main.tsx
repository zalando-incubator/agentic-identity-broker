import React from 'react';
import ReactDOM from 'react-dom/client';
import { ThemeProvider } from '@design-system/theme/ThemeProvider';
import { QueryProvider } from '@services/query/QueryProvider';
import { InlineError } from '@design-system/components/feedback/InlineError/InlineError';
import { DecisionShell } from '@design-system/components/layout/DecisionShell/DecisionShell';
import { commonCopy, navigationCopy } from '@copy';
import App, { AppLoading } from './App';
import './styles/index.css';

const rootElement = document.getElementById('root');
if (!rootElement) throw new Error('Failed to find the root element');

ReactDOM.createRoot(rootElement).render(
  <React.StrictMode>
    <ThemeProvider>
      <QueryProvider
        fallback={<AppLoading />}
        errorFallback={(error) => {
          const unauthorized = typeof error === 'object' && error !== null && 'status' in error && error.status === 401;
          return (
            <DecisionShell wordmarkLabel={commonCopy.brand} skipToMainLabel={navigationCopy.skipToContent}>
              <InlineError message={unauthorized ? commonCopy.authenticationRequired : commonCopy.accountLoadError} onRetry={() => window.location.reload()} retryLabel={commonCopy.retry} />
            </DecisionShell>
          );
        }}
      >
        <App />
      </QueryProvider>
    </ThemeProvider>
  </React.StrictMode>,
);
