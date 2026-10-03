/**
 * Axios transport for the authenticated end-user API.
 * Authentication remains the responsibility of the upstream proxy.
 */

import axios, {
  AxiosError,
  AxiosInstance,
  type InternalAxiosRequestConfig,
} from 'axios';
import type { ApiError } from '../../types/consent';

type AuthLossListener = (error: ApiError) => void;
const authLossListeners = new Set<AuthLossListener>();
let authGeneration = 0;

/** Authentication lifetime for guarding continuations across identity changes. */
export function getAuthGeneration(): number {
  return authGeneration;
}

/** Retire the current lifetime before its principal-owned state is cleared. */
export function advanceAuthGeneration(): void {
  authGeneration += 1;
}

export function subscribeAuthLoss(listener: AuthLossListener): () => void {
  authLossListeners.add(listener);
  return () => { authLossListeners.delete(listener); };
}

/**
 * Create and configure the axios instance with interceptors.
 */
function createApiClient(): AxiosInstance {
  const client = axios.create({
    baseURL: '/api',
    timeout: 30000,
    headers: {
      'Content-Type': 'application/json',
    },
  });
  const requestGenerations = new WeakMap<InternalAxiosRequestConfig, number>();
  client.interceptors.request.use((config) => {
    requestGenerations.set(config, getAuthGeneration());
    return config;
  }, undefined, { synchronous: true });

  // Response interceptor: Handle errors with enhanced error messages
  client.interceptors.response.use(
    (response) => {
      // Success response - return as-is
      return response;
    },
    (error: AxiosError<{ error?: string; message?: string }>) => {
      if (axios.isCancel(error)) return Promise.reject(error);

      // Handle error responses
      if (error.response) {
        const { status, data } = error.response;

        // 401 Unauthorized - Authentication failed
        if (status === 401) {
          console.warn('Unauthorized request - authentication failed');

          // In development, provide more debugging info
          const isDevMode = import.meta.env.DEV;
          const errorMsg = isDevMode
            ? 'Authentication failed. Vite proxy should inject X-Remote-User header automatically.'
            : 'Authentication failed. Please contact your administrator.';

          const authError: ApiError & { retryable: boolean } = {
            status,
            code: 'UNAUTHORIZED',
            message: errorMsg,
            retryable: false,
          };
          if (error.config && requestGenerations.get(error.config) === getAuthGeneration()) {
            advanceAuthGeneration();
            for (const listener of authLossListeners) listener(authError);
          }
          return Promise.reject(authError);
        }

        // 403 Forbidden - User doesn't have permission
        if (status === 403) {
          console.warn('Forbidden request - insufficient permissions');
          return Promise.reject({
            status,
            code: 'FORBIDDEN',
            message: "You don't have permission to access this resource.",
          } as ApiError);
        }

        // 404 Not Found - Resource doesn't exist
        if (status === 404) {
          return Promise.reject({
            status,
            code: 'NOT_FOUND',
            message: 'The requested resource was not found.',
          } as ApiError);
        }

        // 409 Conflict - Resource state conflict (e.g. already actioned)
        if (status === 409) {
          return Promise.reject({
            status,
            code: 'CONFLICT',
            message:
              data?.message ||
              'This resource has already been modified.',
          } as ApiError);
        }

        // 410 Gone - Resource expired or no longer available
        if (status === 410) {
          return Promise.reject({
            status,
            code: 'GONE',
            message: data?.message || 'This resource is no longer available.',
          } as ApiError);
        }

        // 500+ Server Error - Service temporarily unavailable
        if (status >= 500) {
          return Promise.reject({
            status,
            code: 'SERVER_ERROR',
            message: 'Service temporarily unavailable. Please try again later.',
            retryable: true,
          } as ApiError & { retryable: boolean });
        }

        // The end-user API returns { error, message }; consumers receive ApiError.
        if (data && typeof data === 'object' && typeof data.error === 'string' && data.error) {
          return Promise.reject({
            status,
            code: data.error,
            message: typeof data.message === 'string' && data.message ? data.message : error.message || 'An unexpected error occurred',
          } as ApiError);
        }
      }

      // Network error or no response (timeout, connection refused, etc.)
      if (!error.response) {
        const networkError: ApiError & { retryable: boolean } = {
          status: 0,
          code: 'NETWORK_ERROR',
          message:
            'Unable to connect to server. Please check your internet connection.',
          retryable: true,
        };
        return Promise.reject(networkError);
      }

      // Generic error fallback
      const genericError: ApiError = {
        status: error.response?.status || 500,
        code: 'UNKNOWN_ERROR',
        message: error.message || 'An unexpected error occurred',
      };
      return Promise.reject(genericError);
    },
  );

  return client;
}

/**
 * Configured axios instance for making API requests.
 * Use this instance for all API calls in the application.
 *
 * @example
 * ```typescript
 * import { apiClient } from '@services/api/client';
 *
 * const response = await apiClient.get('/consent/agents');
 * console.log(response.data);
 * ```
 */
export const apiClient = createApiClient();

/**
 * Type guard to check if an error is an ApiError.
 */
export function isApiError(error: unknown): error is ApiError {
  return (
    typeof error === 'object' &&
    error !== null &&
    'status' in error &&
    'code' in error &&
    'message' in error
  );
}
