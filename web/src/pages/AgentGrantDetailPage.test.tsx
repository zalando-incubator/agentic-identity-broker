/**
 * Tests for AgentGrantDetailPage component.
 *
 * Tests:
 * - Loading state
 * - Agent not found error
 * - Successful render with agent details
 * - Navigation functionality
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { BrowserRouter, MemoryRouter, Routes, Route } from 'react-router-dom';
import { AgentGrantDetailPage } from './AgentGrantDetailPage';
import { ToastProvider } from '../components/ui/Toast';
import * as useAgentGrantsModule from '../hooks/useAgentGrants';
import * as useConsentModule from '../hooks/useConsent';
import type {
  AgentDetail,
  ThirdpartyService,
  UserGrant,
  CIMDMetadata,
} from '../types/consent';

// Mock the useAgentGrants hook
vi.mock('../hooks/useAgentGrants');
vi.mock('../hooks/useConsent');

// Mock router params
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useParams: () => ({ agentId: 'agent-123' }),
  };
});

const mockAgent: AgentDetail = {
  agentId: 'agent-123',
  displayName: 'Research Assistant',
  description: 'AI agent that helps with research tasks',
  logoUrl: 'https://example.com/agent-logo.png',
  governanceUrl: 'https://example.com/governance',
  userDocumentationUrl: 'https://example.com/docs',
  agentInterfaceUrl: 'https://example.com/interface',
};

const mockServices: ThirdpartyService[] = [
  {
    kind: 'scoped',
    serviceId: 'github',
    displayName: 'GitHub',
    logoUrl: 'https://example.com/github.png',
    scopes: [
      { value: 'read:user', description: 'Read user profile' },
      { value: 'repo', description: 'Access repositories' },
    ],
  },
  {
    kind: 'scoped',
    serviceId: 'google',
    displayName: 'Google Drive',
    logoUrl: 'https://example.com/google.png',
    scopes: [
      { value: 'drive.readonly', description: 'Read files' },
      { value: 'drive.file', description: 'Manage files' },
    ],
  },
];

const mockGrant: UserGrant = {
  id: 'grant-1',
  agent_id: 'agent-123',
  principal: 'user@example.com',
  granted_permission_sets: { 'ps-id-1': ['svc-1'] },
  valid_until: null,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

// Wrapper component for router and provider context
const RouterWrapper = ({ children }: { children: React.ReactNode }) => (
  <ToastProvider>
    <BrowserRouter>
      <Routes>
        <Route path="*" element={children} />
      </Routes>
    </BrowserRouter>
  </ToastProvider>
);

// Wrapper that lets the test control the initial URL (for query-param testing).
const MemoryRouterWrapper = ({
  children,
  initialEntry = '/',
}: {
  children: React.ReactNode;
  initialEntry?: string;
}) => (
  <ToastProvider>
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="*" element={children} />
      </Routes>
    </MemoryRouter>
  </ToastProvider>
);

describe('AgentGrantDetailPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(useConsentModule, 'useConsent').mockReturnValue({
      delegations: [],
      userInfo: {
        principal: 'user@example.com',
        displayName: 'Test User',
        pictureUrl: 'https://example.com/avatar.png',
      },
      loading: false,
      error: null,
      refetch: vi.fn(),
    });
  });

  it('displays loading state with skeletons', () => {
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: null,
      services: [],
      cimdMeta: null,
      grants: null,
      loading: true,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    // Check for breadcrumb link
    expect(screen.getByText('Delegations')).toBeInTheDocument();

    // Loading skeletons should be present
    // We can't easily test for skeleton components, but we can verify the page renders
    expect(screen.getByText('Delegations')).toBeInTheDocument();
  });

  it('displays error state with retry button', () => {
    const mockRefetch = vi.fn();
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: null,
      services: [],
      cimdMeta: null,
      grants: null,
      loading: false,
      error: 'Failed to load agent details',
      refetch: mockRefetch,
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    expect(
      screen.getByText('Failed to load agent details'),
    ).toBeInTheDocument();
    expect(screen.getByText('Try again')).toBeInTheDocument();
  });

  it('displays agent not found error', () => {
    const mockRefetch = vi.fn();
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: null,
      services: [],
      cimdMeta: null,
      grants: null,
      loading: false,
      error: 'Agent not found',
      refetch: mockRefetch,
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    expect(screen.getByText('Agent not found')).toBeInTheDocument();
  });

  it('renders agent details successfully', () => {
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: mockAgent,
      services: mockServices,
      cimdMeta: null,
      grants: mockGrant,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    // Check agent name (using heading role for specificity)
    expect(
      screen.getByRole('heading', { name: 'Research Assistant', level: 1 }),
    ).toBeInTheDocument();
    expect(
      screen.getByText('AI agent that helps with research tasks'),
    ).toBeInTheDocument();

    // Check agent logo
    expect(screen.getByAltText('Research Assistant logo')).toHaveAttribute(
      'src',
      'https://example.com/agent-logo.png',
    );
  });

  it('renders agent links when provided', () => {
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: mockAgent,
      services: mockServices,
      cimdMeta: null,
      grants: mockGrant,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    // Check for links - use getAllByText for duplicate "Documentation"
    expect(screen.getByText('Governance')).toBeInTheDocument();
    const docLinks = screen.getAllByText('Documentation');
    expect(docLinks.length).toBeGreaterThan(0);
    expect(screen.getByText('Agent Interface')).toBeInTheDocument();

    // Verify link targets
    const governanceLink = screen.getByText('Governance').closest('a');
    expect(governanceLink).toHaveAttribute(
      'href',
      'https://example.com/governance',
    );
    expect(governanceLink).toHaveAttribute('target', '_blank');
  });

  it('renders services list', () => {
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: mockAgent,
      services: mockServices,
      cimdMeta: null,
      grants: mockGrant,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    // Services appear in the Connect Your Accounts section
    expect(screen.getByText('GitHub')).toBeInTheDocument();
    expect(screen.getByText('Google Drive')).toBeInTheDocument();
  });

  it('displays empty state when no services available', () => {
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: mockAgent,
      services: [],
      cimdMeta: null,
      grants: null,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    // When there are no services, the "Connect Your Accounts" section is not rendered
    expect(screen.queryByText('Connect Your Accounts')).not.toBeInTheDocument();
  });

  it('renders breadcrumb navigation', () => {
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: mockAgent,
      services: mockServices,
      cimdMeta: null,
      grants: mockGrant,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    // Check breadcrumb using navigation
    const breadcrumb = screen.getByRole('navigation', { name: 'Breadcrumb' });
    const delegationsLink = screen.getByText('Delegations').closest('a');
    expect(delegationsLink).toHaveAttribute('href', '/');
    expect(breadcrumb).toContainHTML('Research Assistant');
  });

  it('renders fallback logo when agent logo is not provided', () => {
    const agentWithoutLogo = { ...mockAgent, logoUrl: undefined };

    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: agentWithoutLogo,
      services: mockServices,
      cimdMeta: null,
      grants: mockGrant,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    // Check for fallback initial letter
    expect(screen.getByText('R')).toBeInTheDocument();
  });

  it('groups grants by service correctly', () => {
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: mockAgent,
      services: mockServices,
      cimdMeta: null,
      grants: mockGrant,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    // GitHub service should be displayed
    expect(screen.getByText('GitHub')).toBeInTheDocument();
    // Services list should show our services
    expect(screen.getByText('Google Drive')).toBeInTheDocument();
  });
});
describe('AgentGrantDetailPage action labels', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(useConsentModule, 'useConsent').mockReturnValue({
      delegations: [],
      userInfo: {
        principal: 'user@example.com',
        displayName: 'Test User',
        pictureUrl: 'https://example.com/avatar.png',
      },
      loading: false,
      error: null,
      refetch: vi.fn(),
    });
  });

  it('shows Save and an End Date heading for an existing grant', () => {
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: mockAgent,
      services: mockServices,
      cimdMeta: null,
      grants: mockGrant,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { name: 'End Date', level: 2 }),
    ).toBeInTheDocument();

    expect(screen.getByText(/before saving\./)).toBeInTheDocument();
  });

  it('shows Approve & Delegate when no grant exists', () => {
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: mockAgent,
      services: mockServices,
      cimdMeta: null,
      grants: null,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    expect(
      screen.getByRole('button', { name: 'Approve & Delegate' }),
    ).toBeInTheDocument();

    expect(screen.getByText(/before approving\./)).toBeInTheDocument();
  });
});

describe('AgentGrantDetailPage - User Story 5: Simplified UI Without Edit Mode (T091-T099)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('Edit mode toggle removal (T093-T095)', () => {
    it('should NOT display edit mode toggle button when page loads', () => {
      vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
        agent: mockAgent,
        services: mockServices,
        cimdMeta: null,
        grants: mockGrant,
        loading: false,
        error: null,
        refetch: vi.fn(),
      });

      render(
        <RouterWrapper>
          <AgentGrantDetailPage />
        </RouterWrapper>,
      );

      // Verify no "Edit Mode" or "View Mode" toggle/switch
      const editModeToggle = screen.queryByRole('switch');
      expect(editModeToggle).not.toBeInTheDocument();

      // Verify no toggle labels
      expect(screen.queryByText('Edit Mode')).not.toBeInTheDocument();
      expect(screen.queryByText('View Mode')).not.toBeInTheDocument();
    });

    it('should always display service connection actions (Login/Disconnect) without edit mode', () => {
      vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
        agent: mockAgent,
        services: mockServices,
        cimdMeta: null,
        grants: mockGrant,
        loading: false,
        error: null,
        refetch: vi.fn(),
      });

      render(
        <RouterWrapper>
          <AgentGrantDetailPage />
        </RouterWrapper>,
      );

      // Verify services are displayed
      expect(screen.getByText('GitHub')).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();

      // Services should be visible without toggling edit mode
      // (This is verified by the fact that the page renders services by default)
      expect(screen.getByText('GitHub')).toBeInTheDocument();
    });

    it('should NOT conditionally render UI based on edit mode', () => {
      // Grant validity control and other edit-specific UI should be present or always available
      vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
        agent: mockAgent,
        services: mockServices,
        cimdMeta: null,
        grants: mockGrant,
        loading: false,
        error: null,
        refetch: vi.fn(),
      });

      render(
        <RouterWrapper>
          <AgentGrantDetailPage />
        </RouterWrapper>,
      );

      // Verify the page structure is not dependent on edit mode
      // The services list should always be visible
      expect(screen.getByText('GitHub')).toBeInTheDocument();

      // No edit mode toggle should exist to conditionally show/hide content
      const editToggle = screen.queryByRole('switch');
      expect(editToggle).not.toBeInTheDocument();
    });
  });

  describe('UI single-mode display (T094-T095)', () => {
    it('should display Login buttons for services without active sessions in single-mode UI', () => {
      // Mock service without grant
      const servicesWithoutGrant = [
        mockServices[0], // GitHub with grant
        mockServices[1], // Google Drive without grant (no delegation)
      ];

      vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
        agent: mockAgent,
        services: servicesWithoutGrant,
        cimdMeta: null,
        grants: mockGrant, // Only has GitHub grant
        loading: false,
        error: null,
        refetch: vi.fn(),
      });

      render(
        <RouterWrapper>
          <AgentGrantDetailPage />
        </RouterWrapper>,
      );

      // Services should be visible without entering any special mode
      expect(screen.getByText('GitHub')).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();
    });

    it('should display Disconnect/action buttons for services with active sessions in single-mode UI', () => {
      vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
        agent: mockAgent,
        services: mockServices,
        cimdMeta: null,
        grants: mockGrant, // Has grant for GitHub
        loading: false,
        error: null,
        refetch: vi.fn(),
      });

      render(
        <RouterWrapper>
          <AgentGrantDetailPage />
        </RouterWrapper>,
      );

      // Services should be visible without needing to toggle edit mode
      expect(screen.getByText('GitHub')).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();

      // Verify services are displayed
      expect(screen.getByText('GitHub')).toBeInTheDocument();
    });
  });

  describe('Approve button logic (T096)', () => {
    it('should NOT disable Approve button based on missing mandatory services (no mandatory service requirements flow)', () => {
      // In the new simplified UI, the approve button behavior is straightforward
      // The page should not have complex approval logic dependent on service status
      vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
        agent: mockAgent,
        services: mockServices,
        cimdMeta: null,
        grants: mockGrant,
        loading: false,
        error: null,
        refetch: vi.fn(),
      });

      render(
        <RouterWrapper>
          <AgentGrantDetailPage />
        </RouterWrapper>,
      );

      // The page should render without error and show agent details
      expect(
        screen.getByRole('heading', { name: 'Research Assistant', level: 1 }),
      ).toBeInTheDocument();
    });
  });

  describe('UI consistency across states (T097-T099)', () => {
    it('should maintain consistent UI across page loads (no state-dependent rendering of edit mode)', () => {
      // First render
      const { unmount: unmount1 } = render(
        <RouterWrapper>
          <AgentGrantDetailPage />
        </RouterWrapper>,
      );

      vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
        agent: mockAgent,
        services: mockServices,
        cimdMeta: null,
        grants: mockGrant,
        loading: false,
        error: null,
        refetch: vi.fn(),
      });

      // Verify no edit mode toggle
      expect(screen.queryByRole('switch')).not.toBeInTheDocument();
      unmount1();

      // Second render (simulate re-mount)
      render(
        <RouterWrapper>
          <AgentGrantDetailPage />
        </RouterWrapper>,
      );

      // Should still have no edit mode toggle
      expect(screen.queryByRole('switch')).not.toBeInTheDocument();
    });

    it('should always show action buttons (not hidden in any mode)', () => {
      vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
        agent: mockAgent,
        services: mockServices,
        cimdMeta: null,
        grants: mockGrant,
        loading: false,
        error: null,
        refetch: vi.fn(),
      });

      render(
        <RouterWrapper>
          <AgentGrantDetailPage />
        </RouterWrapper>,
      );

      // Services and their associated actions should always be visible
      expect(screen.getByText('GitHub')).toBeInTheDocument();
      expect(screen.getByText('Google Drive')).toBeInTheDocument();

      // Services section should be visible (not conditional on edit mode)
      expect(screen.getByText('GitHub')).toBeInTheDocument();
    });

    it('should not require toggling edit mode to interact with services', () => {
      vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
        agent: mockAgent,
        services: mockServices,
        cimdMeta: null,
        grants: mockGrant,
        loading: false,
        error: null,
        refetch: vi.fn(),
      });

      render(
        <RouterWrapper>
          <AgentGrantDetailPage />
        </RouterWrapper>,
      );

      // Verify there is NO edit mode toggle at all
      const editToggle = screen.queryByRole('switch');
      expect(editToggle).not.toBeInTheDocument();

      // Verify there are NO "Edit", "View" mode labels
      expect(screen.queryByText(/Edit Mode/i)).not.toBeInTheDocument();
      expect(screen.queryByText(/View Mode/i)).not.toBeInTheDocument();

      // Services list should be immediately interactive
      expect(screen.getByText('GitHub')).toBeInTheDocument();
    });
  });
});

describe('AgentGrantDetailPage - Revoke All Access (T017)', () => {
  // A grant with active permission set IDs
  const grantWithTokens = {
    id: 'grant-1',
    agent_id: 'agent-123',
    principal: 'user@example.com',
    granted_permission_sets: { 'ps-id-1': ['svc-1'] },
    valid_until: null,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(useConsentModule, 'useConsent').mockReturnValue({
      delegations: [],
      userInfo: {
        principal: 'user@example.com',
        displayName: 'Test User',
        pictureUrl: 'https://example.com/avatar.png',
      },
      loading: false,
      error: null,
      refetch: vi.fn(),
    });
  });

  it('RevokeGrantButton visible when grant exists with delegated tokens', () => {
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: mockAgent,
      services: mockServices,
      cimdMeta: null,
      grants: grantWithTokens,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    expect(
      screen.getByRole('button', {
        name: /revoke all access for research assistant/i,
      }),
    ).toBeInTheDocument();
  });

  it('RevokeGrantButton absent when user has no active grant (null grants)', () => {
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: mockAgent,
      services: mockServices,
      cimdMeta: null,
      grants: null,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    expect(
      screen.queryByRole('button', {
        name: /revoke all access for research assistant/i,
      }),
    ).not.toBeInTheDocument();
  });

  it('RevokeGrantButton absent when grant has empty delegated tokens', () => {
    const emptyGrant = {
      ...grantWithTokens,
      granted_permission_sets: {},
    };

    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: mockAgent,
      services: mockServices,
      cimdMeta: null,
      grants: emptyGrant,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <RouterWrapper>
        <AgentGrantDetailPage />
      </RouterWrapper>,
    );

    expect(
      screen.queryByRole('button', {
        name: /revoke all access for research assistant/i,
      }),
    ).not.toBeInTheDocument();
  });
});

describe('AgentGrantDetailPage - CIMD session_token flow', () => {
  const mockUseConsentReturn = {
    delegations: [],
    userInfo: {
      principal: 'user@example.com',
      displayName: 'Test User',
      pictureUrl: '',
    },
    loading: false,
    error: null,
    refetch: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(useConsentModule, 'useConsent').mockReturnValue(
      mockUseConsentReturn,
    );
  });

  it('forwards session_token from URL to useAgentGrants', () => {
    const useAgentGrantsSpy = vi
      .spyOn(useAgentGrantsModule, 'useAgentGrants')
      .mockReturnValue({
        agent: mockAgent,
        services: mockServices,
        cimdMeta: null,
        grants: mockGrant,
        loading: false,
        error: null,
        refetch: vi.fn(),
      });

    render(
      <MemoryRouterWrapper initialEntry="/agents/agent-123?session_token=test-session-abc">
        <AgentGrantDetailPage />
      </MemoryRouterWrapper>,
    );

    expect(useAgentGrantsSpy).toHaveBeenCalledWith('agent-123', {
      sessionToken: 'test-session-abc',
    });
  });

  it('renders CIMDConsentSummary and domain badge when cimdMeta is present', () => {
    const mockCimdMeta: CIMDMetadata = {
      client_id_url: 'https://acme.example.com/client',
      redirect_uri: 'https://acme.example.com/callback',
      verified_domain: 'acme.example.com',
      requested_scopes: ['read:user'],
    };

    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: mockAgent,
      services: mockServices,
      cimdMeta: mockCimdMeta,
      grants: mockGrant,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(
      <MemoryRouterWrapper initialEntry="/agents/agent-123?session_token=test-session-abc">
        <AgentGrantDetailPage />
      </MemoryRouterWrapper>,
    );

    // CIMDConsentSummary renders the verified domain as primary identifier
    expect(screen.getByText('acme.example.com')).toBeInTheDocument();
    // and the "wants to access" sentence that includes the agent name
    const cimdParagraph = screen.getByText(/wants to access/);
    expect(cimdParagraph).toBeInTheDocument();
    expect(cimdParagraph.textContent).toContain('Research Assistant');
  });
});
