/**
 * Tests for ServiceRequirementCard component.
 *
 * Tests mandatory vs optional service display, connection status,
 * scope list rendering, and button interactions.
 */

import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ServiceRequirementCard } from './ServiceRequirementCard';

describe('ServiceRequirementCard', () => {
  const mockOnLogin = vi.fn();
  const mockOnDisconnect = vi.fn();

  const mockScopes = [
    {
      name: 'read:email',
      description: 'Read your email address',
    },
    {
      name: 'read:profile',
      description: 'Read your profile information',
    },
  ];

  beforeEach(() => {
    mockOnLogin.mockClear();
    mockOnDisconnect.mockClear();
  });

  describe('Rendering mandatory service', () => {
    it('renders mandatory service with "Required" badge in trust-deep color', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      // Service name should be visible
      expect(screen.getByText('GitHub')).toBeInTheDocument();

      // Required badge should be visible
      const requiredBadge = screen.getByText('Required');
      expect(requiredBadge).toBeInTheDocument();

      // Badge should use primary variant (trust-deep color)
      expect(requiredBadge.closest('span')).toHaveClass('bg-trust-deep');
    });
  });

  describe('Rendering optional service', () => {
    it('renders optional service with "Optional" badge in neutral color', () => {
      render(
        <ServiceRequirementCard
          serviceId="stripe"
          serviceName="Stripe"
          requirementType="optional"
          requiredScopes={mockScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      // Service name should be visible
      expect(screen.getByText('Stripe')).toBeInTheDocument();

      // Optional badge should be visible
      const optionalBadge = screen.getByText('Optional');
      expect(optionalBadge).toBeInTheDocument();

      // Badge should use neutral variant
      expect(optionalBadge.closest('span')).toHaveClass('bg-neutral-100');
    });
  });

  describe('Service name and requirement type display', () => {
    it('displays service name and connection status', () => {
      render(
        <ServiceRequirementCard
          serviceId="google"
          serviceName="Google Drive"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      expect(screen.getByText('Google Drive')).toBeInTheDocument();
    });
  });

  describe('Connection status display', () => {
    it('shows "Login" button when not connected', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      const loginButton = screen.getByRole('button', { name: /login/i });
      expect(loginButton).toBeInTheDocument();
    });

    it('shows "Active Session" badge when connected', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      const activeBadge = screen.getByText('Active Session');
      expect(activeBadge).toBeInTheDocument();

      // Should use success variant
      expect(activeBadge.closest('span')).toHaveClass('bg-success-light');
    });
  });

  describe('Scope list display (read-only)', () => {
    it('displays scope list (read-only)', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      // Should show scope names
      expect(screen.getByText('read:email')).toBeInTheDocument();
      expect(screen.getByText('read:profile')).toBeInTheDocument();

      // Should show scope descriptions
      expect(screen.getByText('Read your email address')).toBeInTheDocument();
      expect(
        screen.getByText('Read your profile information'),
      ).toBeInTheDocument();

      // Should have "Required Permissions:" label
      expect(screen.getByText('Required Permissions:')).toBeInTheDocument();
    });

    it('displays scope names in code tags with trust-deep styling', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      // Find all code elements
      const codeElements = screen.getAllByText(/read:(email|profile)/);
      codeElements.forEach((element) => {
        // Should be in a code tag
        expect(element.tagName).toBe('CODE');
        // Should have font-mono class for code styling
        expect(element).toHaveClass('font-mono');
        // Should have trust-deep color
        expect(element).toHaveClass('text-trust-deep');
      });
    });

    it('displays scope descriptions as helper text below scope names', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      // Each scope should have its description displayed
      expect(screen.getByText('Read your email address')).toBeInTheDocument();
      expect(
        screen.getByText('Read your profile information'),
      ).toBeInTheDocument();

      // Verify descriptions are displayed as helper text (smaller text)
      const descriptions = screen.getAllByText(/Read your (email|profile)/);
      descriptions.forEach((element) => {
        // Should use text-xs for smaller text
        expect(element).toHaveClass('text-xs');
        // Should use neutral-600 or similar gray for helper text
        expect(element.className).toMatch(/text-neutral/);
      });
    });

    it('handles empty scope list gracefully', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={[]}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      // Should still render with empty scopes
      expect(screen.getByText('GitHub')).toBeInTheDocument();
      expect(screen.getByText('Required Permissions:')).toBeInTheDocument();
    });

    it('displays scope descriptions when provided', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      expect(screen.getByText('Read your email address')).toBeInTheDocument();
      expect(
        screen.getByText('Read your profile information'),
      ).toBeInTheDocument();
    });

    it('handles scopes without descriptions (display name only)', () => {
      const scopesWithoutDesc = [
        { name: 'read:email' },
        { name: 'write:profile', description: 'Write profile data' },
      ];

      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={scopesWithoutDesc}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      // Should display both scope names
      expect(screen.getByText('read:email')).toBeInTheDocument();
      expect(screen.getByText('write:profile')).toBeInTheDocument();

      // read:email should NOT have a description paragraph (verify it's truly read-only with no description)
      const emailCodeElement = screen.getByText('read:email');
      const emailContainer = emailCodeElement.closest('div');
      expect(emailContainer).not.toHaveTextContent('Read your email');

      // write:profile SHOULD have its description
      expect(screen.getByText('Write profile data')).toBeInTheDocument();
    });

    it('renders scopes as read-only display (not interactive)', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      // Verify no checkboxes or toggles for scopes
      const checkboxes = screen.queryAllByRole('checkbox');
      const toggles = screen.queryAllByRole('switch');
      const radioButtons = screen.queryAllByRole('radio');

      expect(checkboxes).toHaveLength(0);
      expect(toggles).toHaveLength(0);
      expect(radioButtons).toHaveLength(0);

      // Verify scope elements are not interactive (no input elements)
      const codeElements = screen.getAllByText(/read:(email|profile)/);
      codeElements.forEach((element) => {
        const container = element.closest('div');
        if (container) {
          const inputs = container.querySelectorAll('input');
          expect(inputs).toHaveLength(0);
        }
      });
    });

    it('displays all required scopes under service with descriptions', () => {
      const multipleScopes = [
        { name: 'read:email', description: 'Access email address' },
        { name: 'read:profile', description: 'Access profile information' },
        { name: 'write:gists', description: 'Create and update gists' },
      ];

      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={multipleScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      // All scopes should be visible
      expect(screen.getByText('read:email')).toBeInTheDocument();
      expect(screen.getByText('read:profile')).toBeInTheDocument();
      expect(screen.getByText('write:gists')).toBeInTheDocument();

      // All descriptions should be visible
      expect(screen.getByText('Access email address')).toBeInTheDocument();
      expect(
        screen.getByText('Access profile information'),
      ).toBeInTheDocument();
      expect(screen.getByText('Create and update gists')).toBeInTheDocument();
    });
  });

  describe('User interactions', () => {
    it('calls onLogin when Login button is clicked', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      const loginButton = screen.getByRole('button', { name: /login/i });
      fireEvent.click(loginButton);

      expect(mockOnLogin).toHaveBeenCalledWith('github');
      expect(mockOnLogin).toHaveBeenCalledTimes(1);
    });

    it('shows Disconnect button when connected', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      const disconnectButton = screen.getByRole('button', {
        name: /disconnect/i,
      });
      expect(disconnectButton).toBeInTheDocument();
    });

    it('calls onDisconnect when Disconnect button is clicked', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      const disconnectButton = screen.getByRole('button', {
        name: /disconnect/i,
      });
      fireEvent.click(disconnectButton);

      expect(mockOnDisconnect).toHaveBeenCalledWith('github');
      expect(mockOnDisconnect).toHaveBeenCalledTimes(1);
    });

    it('does not show Disconnect button when not connected', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      const disconnectButton = screen.queryByRole('button', {
        name: /disconnect/i,
      });
      expect(disconnectButton).not.toBeInTheDocument();
    });

    it('does not show Login button when connected', () => {
      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={mockScopes}
          connectionStatus="connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      const loginButton = screen.queryByRole('button', { name: /login/i });
      expect(loginButton).not.toBeInTheDocument();
    });
  });

  describe('Edge cases', () => {
    it('handles optional service with long service name', () => {
      render(
        <ServiceRequirementCard
          serviceId="service-id"
          serviceName="Very Long Service Name That Should Still Display Properly"
          requirementType="optional"
          requiredScopes={mockScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      expect(
        screen.getByText(
          'Very Long Service Name That Should Still Display Properly',
        ),
      ).toBeInTheDocument();
    });

    it('handles scope names with special characters', () => {
      const specialScopes = [
        { name: 'read:repo:status', description: 'Read repository status' },
        { name: 'admin:repo_hook', description: 'Administer repository hooks' },
      ];

      render(
        <ServiceRequirementCard
          serviceId="github"
          serviceName="GitHub"
          requirementType="mandatory"
          requiredScopes={specialScopes}
          connectionStatus="not_connected"
          onLogin={mockOnLogin}
          onDisconnect={mockOnDisconnect}
        />,
      );

      expect(screen.getByText('read:repo:status')).toBeInTheDocument();
      expect(screen.getByText('admin:repo_hook')).toBeInTheDocument();
    });
  });
});
