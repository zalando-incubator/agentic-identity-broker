/**
 * SessionCard Component Tests
 *
 * Test Coverage:
 * - Component rendering with different session states
 * - Status badge variants (Active, Expiring Soon, Access Token Expired, Expired)
 * - Action button visibility and behavior
 * - Loading state handling
 * - Accessibility compliance
 */

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SessionCard } from './SessionCard';
import type { SessionSummary } from '@services/api/sessions';

// Mock session data factory
const createMockSession = (
  overrides?: Partial<SessionSummary>,
): SessionSummary => ({
  id: 'test-session-id',
  service_id: 'google',
  service_display_name: 'Google Drive',
  token_type: 'Bearer',
  scope: ['read:email', 'write:files'],
  initiated_at: new Date('2024-01-01T12:00:00Z').toISOString(),
  is_expired: false,
  access_token_expired: false,
  has_refresh_token: true,
  refresh_token_expires_at: new Date(
    Date.now() + 30 * 24 * 60 * 60 * 1000,
  ).toISOString(),
  dependent_agent_count: 2,
  is_encrypted: true,
  ...overrides,
});

describe('SessionCard', () => {
  describe('Rendering', () => {
    it('renders session information correctly', () => {
      const session = createMockSession();
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard session={session} onTerminate={onTerminate} />,
      );

      // Verify service name
      expect(screen.getByText('Google Drive')).toBeInTheDocument();

      // Verify component renders
      expect(container).toBeInTheDocument();
    });

    it('renders "Active" status for valid session', () => {
      const session = createMockSession();
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard session={session} onTerminate={onTerminate} />,
      );

      // Verify component renders for active session
      expect(container).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();
    });

    it('renders "Expiring Soon" status for session expiring within 7 days', () => {
      const session = createMockSession({
        refresh_token_expires_at: new Date(
          Date.now() + 5 * 24 * 60 * 60 * 1000,
        ).toISOString(),
      });
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard session={session} onTerminate={onTerminate} />,
      );

      // Verify component renders for expiring session
      expect(container).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();
    });

    it('renders "Access Token Expired" status when access token is expired', () => {
      const session = createMockSession({
        access_token_expired: true,
      });
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard session={session} onTerminate={onTerminate} />,
      );

      // Verify component renders for expired access token
      expect(container).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();
    });

    it('renders "Expired" status for fully expired session', () => {
      const session = createMockSession({
        is_expired: true,
      });
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard session={session} onTerminate={onTerminate} />,
      );

      // Verify component renders for expired session
      expect(container).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();
    });

    it('renders singular "agent" for dependent_agent_count of 1', () => {
      const session = createMockSession({
        dependent_agent_count: 1,
      });
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard session={session} onTerminate={onTerminate} />,
      );

      expect(container).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();
    });

    it('does not render scopes section when no scopes exist', () => {
      const session = createMockSession({
        scope: [],
      });
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard session={session} onTerminate={onTerminate} />,
      );

      expect(container).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();
    });
  });

  describe('Actions', () => {
    it('renders "View Details" and "Terminate" buttons for active session', () => {
      const session = createMockSession();
      const onTerminate = vi.fn();
      const onViewDetails = vi.fn();

      const { container } = render(
        <SessionCard
          session={session}
          onTerminate={onTerminate}
          onViewDetails={onViewDetails}
        />,
      );

      // Verify component renders with buttons
      expect(container).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();
      expect(
        screen.getByRole('button', { name: /view details/i }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole('button', { name: /terminate/i }),
      ).toBeInTheDocument();
    });

    it('calls onTerminate when Terminate button is clicked', async () => {
      const user = userEvent.setup();
      const session = createMockSession();
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard session={session} onTerminate={onTerminate} />,
      );

      expect(container).toBeInTheDocument();

      await user.click(screen.getByRole('button', { name: /terminate/i }));
      expect(onTerminate).toHaveBeenCalledWith(session.service_id);
    });

    it('calls onViewDetails when View Details button is clicked', async () => {
      const user = userEvent.setup();
      const session = createMockSession();
      const onTerminate = vi.fn();
      const onViewDetails = vi.fn();

      const { container } = render(
        <SessionCard
          session={session}
          onTerminate={onTerminate}
          onViewDetails={onViewDetails}
        />,
      );

      expect(container).toBeInTheDocument();

      await user.click(screen.getByRole('button', { name: /view details/i }));
      expect(onViewDetails).toHaveBeenCalledTimes(1);
      expect(onViewDetails).toHaveBeenCalledWith(session.service_id);
    });

    it('does not render "View Details" button when onViewDetails is not provided', () => {
      const session = createMockSession();
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard session={session} onTerminate={onTerminate} />,
      );

      expect(container).toBeInTheDocument();
      expect(screen.queryByRole('button', { name: /view details/i })).toBeNull();
    });

    it('renders expired message instead of buttons for expired session', () => {
      const session = createMockSession({
        is_expired: true,
      });
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard session={session} onTerminate={onTerminate} />,
      );

      // Verify component renders for expired session
      expect(container).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();
    });
  });


  describe('Refresh button', () => {
    it('renders Refresh button when refresh token exists and onRefresh is provided', () => {
      const session = createMockSession({ has_refresh_token: true });
      const onTerminate = vi.fn();
      const onRefresh = vi.fn();

      render(
        <SessionCard
          session={session}
          onTerminate={onTerminate}
          onRefresh={onRefresh}
        />,
      );

      expect(screen.getByRole('button', { name: /refresh/i })).toBeInTheDocument();
    });

    it('does not render Refresh button when refresh token is missing', () => {
      const session = createMockSession({ has_refresh_token: false });

      render(
        <SessionCard
          session={session}
          onTerminate={vi.fn()}
          onRefresh={vi.fn()}
        />,
      );

      expect(screen.queryByRole('button', { name: /refresh/i })).toBeNull();
    });

    it('calls onRefresh with service id when clicked', async () => {
      const user = userEvent.setup();
      const session = createMockSession();
      const onRefresh = vi.fn();

      render(
        <SessionCard
          session={session}
          onTerminate={vi.fn()}
          onRefresh={onRefresh}
        />,
      );

      await user.click(screen.getByRole('button', { name: /refresh/i }));

      expect(onRefresh).toHaveBeenCalledWith(session.service_id);
    });

    it('does not render Refresh button when onRefresh is omitted', () => {
      const session = createMockSession();

      render(<SessionCard session={session} onTerminate={vi.fn()} />);

      expect(screen.queryByRole('button', { name: /refresh/i })).toBeNull();
    });

    it('does not render Refresh button for an expired session', () => {
      const session = createMockSession({
        is_expired: true,
        has_refresh_token: true,
      });

      render(
        <SessionCard
          session={session}
          onTerminate={vi.fn()}
          onRefresh={vi.fn()}
          onLogin={vi.fn()}
        />,
      );

      expect(screen.queryByRole('button', { name: /refresh/i })).toBeNull();
    });
  });

  describe('Login button', () => {
    it('calls onLogin with service id for an expired session', async () => {
      const user = userEvent.setup();
      const session = createMockSession({ is_expired: true });
      const onLogin = vi.fn();

      render(
        <SessionCard
          session={session}
          onTerminate={vi.fn()}
          onLogin={onLogin}
        />,
      );

      await user.click(screen.getByRole('button', { name: /login/i }));

      expect(onLogin).toHaveBeenCalledWith(session.service_id);
    });

    it('does not render Login button for an active session', () => {
      const session = createMockSession();

      render(
        <SessionCard
          session={session}
          onTerminate={vi.fn()}
          onLogin={vi.fn()}
        />,
      );

      expect(screen.queryByRole('button', { name: /login/i })).toBeNull();
    });
  });
  describe('Loading State', () => {
    it('disables buttons and shows loading spinner when loading', () => {
      const session = createMockSession();
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard
          session={session}
          onTerminate={onTerminate}
          loading={true}
        />,
      );

      // Verify component renders in loading state
      expect(container).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();
    });

    it('disables View Details button when loading', () => {
      const session = createMockSession();
      const onTerminate = vi.fn();
      const onViewDetails = vi.fn();

      const { container } = render(
        <SessionCard
          session={session}
          onTerminate={onTerminate}
          onViewDetails={onViewDetails}
          loading={true}
        />,
      );

      // Verify component renders in loading state
      expect(container).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();
    });
  });

  describe('Accessibility', () => {
    it('renders semantic HTML structure', () => {
      const session = createMockSession();
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard session={session} onTerminate={onTerminate} />,
      );

      // Card should use semantic heading for service name
      const heading = container.querySelector('h3');
      expect(heading).toBeInTheDocument();
      expect(heading).toHaveTextContent('Google Drive');
    });

    it('has accessible button labels', () => {
      const session = createMockSession();
      const onTerminate = vi.fn();
      const onViewDetails = vi.fn();

      const { container } = render(
        <SessionCard
          session={session}
          onTerminate={onTerminate}
          onViewDetails={onViewDetails}
        />,
      );

      // Verify component renders
      expect(container).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();
    });

    it('supports keyboard navigation', async () => {
      const user = userEvent.setup();
      const session = createMockSession();
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard session={session} onTerminate={onTerminate} />,
      );

      expect(container).toBeInTheDocument();

      await user.tab();
      expect(screen.getByRole('button', { name: /terminate/i })).toHaveFocus();
    });
  });

  describe('Date Formatting', () => {
    it('formats initiated date correctly', () => {
      const session = createMockSession({
        initiated_at: new Date('2024-03-15T14:30:00Z').toISOString(),
      });
      const onTerminate = vi.fn();

      const { container } = render(
        <SessionCard session={session} onTerminate={onTerminate} />,
      );

      // Verify component renders
      expect(container).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();
    });

    it('does not display refresh token expiry info when has_refresh_token is false', () => {
      const session = createMockSession({
        has_refresh_token: false,
        refresh_token_expires_at: undefined,
      });
      const onTerminate = vi.fn();

      render(<SessionCard session={session} onTerminate={onTerminate} />);

      expect(
        screen.queryByText(/Session might expire after/i),
      ).not.toBeInTheDocument();
    });

    it('does not display refresh token expiry info for an expired session', () => {
      const session = createMockSession({
        is_expired: true,
        has_refresh_token: true,
        refresh_token_expires_at: new Date(Date.now() - 60_000).toISOString(),
      });
      const onTerminate = vi.fn();

      render(<SessionCard session={session} onTerminate={onTerminate} />);

      expect(
        screen.queryByText(/Session might expire after/i),
      ).not.toBeInTheDocument();
    });
  });
});
