import { Component, type ErrorInfo, type ReactNode } from 'react';
import { Alert } from '../Alert/Alert';
import { Button } from '@design-system/components/primitives/Button';

export interface ErrorBoundaryProps {
  children: ReactNode;
  title: string;
  description?: string;
  retryLabel: string;
  fallback?: ReactNode | ((error: Error, reset: () => void) => ReactNode);
  onError?: (error: Error, info: ErrorInfo) => void;
  onReset?: () => void;
  /** Change a key when navigation or external recovery makes a retry meaningful. */
  resetKeys?: readonly unknown[];
}

interface ErrorBoundaryState { error: Error | null }

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    this.props.onError?.(error, info);
  }

  componentDidUpdate(previous: ErrorBoundaryProps, previousState: ErrorBoundaryState) {
    const before = previous.resetKeys ?? [];
    const after = this.props.resetKeys ?? [];
    if (this.state.error && previousState.error && (before.length !== after.length || before.some((key, index) => !Object.is(key, after[index])))) {
      this.resetErrorBoundary();
    }
  }

  resetErrorBoundary = () => {
    this.props.onReset?.();
    this.setState({ error: null });
  };

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    const { fallback, title, description, retryLabel } = this.props;
    if (fallback !== undefined) return typeof fallback === 'function' ? fallback(error, this.resetErrorBoundary) : fallback;
    return <Alert variant="error" title={title}>
      {description && <p>{description}</p>}
      <Button variant="outline" className="mt-3" onClick={this.resetErrorBoundary}>{retryLabel}</Button>
    </Alert>;
  }
}

export default ErrorBoundary;
