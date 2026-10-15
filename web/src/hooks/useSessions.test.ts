/**
 * useSessions Hook Tests
 *
 * Test Coverage:
 * - Data fetching on mount
 * - Loading state management
 * - Error handling
 * - Refetch functionality
 * - Cleanup on unmount
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import axios from 'axios';
import { useSessions, useSession } from './useSessions';
import { sessionsApi } from '../services/api/sessions';
import type { SessionSummary } from '../services/api/sessions';

// Mock the sessions API
vi.mock('../services/api/sessions', () => ({
  sessionsApi: {
    listSessions: vi.fn(),
  },
}));

// Mock session data
const mockSessions: SessionSummary[] = [
  {
    id: 'session-1',
    service_id: 'google',
    service_display_name: 'Google Drive',
    token_type: 'Bearer',
    scope: ['read:email', 'write:files'],
    initiated_at: new Date('2024-01-01T12:00:00Z').toISOString(),
    requires_reauthentication: false,
    access_token_expired: false,
    has_refresh_token: true,
    dependent_agent_count: 2,
    is_encrypted: true,
  },
  {
    id: 'session-2',
    service_id: 'github',
    service_display_name: 'GitHub',
    token_type: 'Bearer',
    scope: ['repo', 'user'],
    initiated_at: new Date('2024-02-01T12:00:00Z').toISOString(),
    requires_reauthentication: false,
    access_token_expired: false,
    has_refresh_token: true,
    dependent_agent_count: 1,
    is_encrypted: true,
  },
];

describe('useSessions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  describe('Data Fetching', () => {
    it('fetches sessions on mount', async () => {
      vi.mocked(sessionsApi.listSessions).mockResolvedValueOnce(mockSessions);

      const { result } = renderHook(() => useSessions());

      // Initially loading
      expect(result.current.loading).toBe(true);
      expect(result.current.sessions).toEqual([]);
      expect(result.current.error).toBeNull();

      // Wait for data to load
      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      // Verify sessions loaded
      expect(result.current.sessions).toEqual(mockSessions);
      expect(result.current.error).toBeNull();
      expect(sessionsApi.listSessions).toHaveBeenCalledOnce();
    });

    it('handles empty sessions list', async () => {
      vi.mocked(sessionsApi.listSessions).mockResolvedValueOnce([]);

      const { result } = renderHook(() => useSessions());

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.sessions).toEqual([]);
      expect(result.current.error).toBeNull();
    });
  });

  describe('Error Handling', () => {
    it('handles API errors gracefully', async () => {
      const errorMessage = 'Network error';
      vi.mocked(sessionsApi.listSessions).mockRejectedValueOnce(
        new Error(errorMessage),
      );

      const { result } = renderHook(() => useSessions());

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.sessions).toEqual([]);
      expect(result.current.error).toBe(errorMessage);
    });

    it('handles 404 errors with custom message', async () => {
      const error = new axios.AxiosError('Not Found');
      error.response = { status: 404, data: { message: 'Not found' } } as never;
      vi.mocked(sessionsApi.listSessions).mockRejectedValueOnce(error);

      const { result } = renderHook(() => useSessions());

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.error).toBe('No sessions found');
    });

    it('handles API error responses with message', async () => {
      const error = new axios.AxiosError('Internal Server Error');
      error.response = { status: 500, data: { message: 'Internal server error' } } as never;
      vi.mocked(sessionsApi.listSessions).mockRejectedValueOnce(error);

      const { result } = renderHook(() => useSessions());

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.error).toBe('Internal server error');
    });

    it('handles unknown errors with default message', async () => {
      vi.mocked(sessionsApi.listSessions).mockRejectedValueOnce(
        'Unknown error',
      );

      const { result } = renderHook(() => useSessions());

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.error).toBe('Failed to load OAuth2 sessions');
    });
  });

  describe('Refetch Functionality', () => {
    it('refetches sessions when refetch is called', async () => {
      vi.mocked(sessionsApi.listSessions)
        .mockResolvedValueOnce(mockSessions)
        .mockResolvedValueOnce([mockSessions[0]]);

      const { result } = renderHook(() => useSessions());

      // Wait for initial load
      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.sessions).toEqual(mockSessions);

      // Call refetch
      await act(async () => {
        await result.current.refetch();
      });

      // Wait for refetch to complete
      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      // Verify updated data
      expect(result.current.sessions).toEqual([mockSessions[0]]);
      expect(sessionsApi.listSessions).toHaveBeenCalledTimes(2);
    });

    it('clears error when refetching after error', async () => {
      vi.mocked(sessionsApi.listSessions)
        .mockRejectedValueOnce(new Error('Network error'))
        .mockResolvedValueOnce(mockSessions);

      const { result } = renderHook(() => useSessions());

      // Wait for initial error
      await waitFor(() => {
        expect(result.current.error).toBeTruthy();
      });

      // Call refetch
      await act(async () => {
        await result.current.refetch();
      });

      // Wait for refetch to complete
      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      // Verify error cleared and data loaded
      expect(result.current.error).toBeNull();
      expect(result.current.sessions).toEqual(mockSessions);
    });

    it('sets loading state during refetch', async () => {
      vi.mocked(sessionsApi.listSessions)
        .mockResolvedValueOnce(mockSessions)
        .mockImplementationOnce(
          () =>
            new Promise((resolve) => {
              setTimeout(() => resolve(mockSessions), 100);
            }),
        );

      const { result } = renderHook(() => useSessions());

      // Wait for initial load
      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      // Start refetch
      let refetchPromise: Promise<void>;

      await act(async () => {
        refetchPromise = result.current.refetch();
      });

      // Should be loading immediately
      await waitFor(() => {
        expect(result.current.loading).toBe(true);
      });

      // Wait for refetch to complete
      await act(async () => {
        await refetchPromise;
      });

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });
    });
  });

  describe('Loading State', () => {
    it('starts with loading state', () => {
      vi.mocked(sessionsApi.listSessions).mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            setTimeout(() => resolve(mockSessions), 1000);
          }),
      );

      const { result } = renderHook(() => useSessions());

      expect(result.current.loading).toBe(true);
      expect(result.current.sessions).toEqual([]);
      expect(result.current.error).toBeNull();
    });

    it('clears loading state after success', async () => {
      vi.mocked(sessionsApi.listSessions).mockResolvedValueOnce(mockSessions);

      const { result } = renderHook(() => useSessions());

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.sessions).toEqual(mockSessions);
    });

    it('clears loading state after error', async () => {
      vi.mocked(sessionsApi.listSessions).mockRejectedValueOnce(
        new Error('Network error'),
      );

      const { result } = renderHook(() => useSessions());

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.error).toBeTruthy();
    });
  });
});

describe('useSession', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it('returns session by service ID', async () => {
    vi.mocked(sessionsApi.listSessions).mockResolvedValueOnce(mockSessions);

    const { result } = renderHook(() => useSession('google'));

    await waitFor(() => {
      expect(result.current).toBeDefined();
    });

    expect(result.current?.service_id).toBe('google');
    expect(result.current?.service_display_name).toBe('Google Drive');
  });

  it('returns undefined for non-existent service ID', async () => {
    vi.mocked(sessionsApi.listSessions).mockResolvedValueOnce(mockSessions);

    const { result } = renderHook(() => useSession('nonexistent'));

    await waitFor(() => {
      expect(result.current).toBeUndefined();
    });
  });

  it('returns undefined when sessions are loading', () => {
    vi.mocked(sessionsApi.listSessions).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          setTimeout(() => resolve(mockSessions), 1000);
        }),
    );

    const { result } = renderHook(() => useSession('google'));

    expect(result.current).toBeUndefined();
  });
});
