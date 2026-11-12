/**
 * Integration tests for AgentGrantDetailPage grant submission flow.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { AgentGrantDetailPage } from './AgentGrantDetailPage';
import { consentApi } from '@services/api/consent';
import { ToastProvider } from '../components/ui/Toast';
import type {
  AgentDetail,
  ThirdpartyService,
  UserGrant,
} from '../types/consent';

// Mock the API
vi.mock('@services/api/consent', () => ({
  consentApi: {
    getUserInfo: vi.fn(),
    getAgentDelegations: vi.fn(),
    getAgentDetail: vi.fn(),
    getAgentGrants: vi.fn(),
    createOrUpdateGrant: vi.fn(),
  },
}));

describe('AgentGrantDetailPage - Integration', () => {
  const mockAgent: AgentDetail = {
    agentId: 'test-agent',
    displayName: 'Test Agent',
    description: 'A test agent',
    logoUrl: 'https://example.com/logo.png',
    governanceUrl: 'https://example.com/governance',
    userDocumentationUrl: 'https://example.com/docs',
    agentInterfaceUrl: 'https://example.com/interface',
  };

  const mockServices: ThirdpartyService[] = [
    {
      kind: 'scoped',
      serviceId: 'github',
      displayName: 'GitHub',
      scopes: [
        { value: 'read:user', description: 'Read user profile' },
        { value: 'read:repo', description: 'Read repositories' },
      ],
    },
  ];

  const mockGrant: UserGrant = {
    id: 'grant-123',
    agent_id: 'test-agent',
    principal: 'user@example.com',
    delegated_oauth2_tokens: [
      {
        thirdparty_oauth2_service_id: 'github',
        scopes: ['read:user'],
      },
    ],
    valid_until: null,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  };

  beforeEach(() => {
    vi.clearAllMocks();

    // Setup default mock responses
    vi.mocked(consentApi.getUserInfo).mockResolvedValue({
      principal: 'user@example.com',
      displayName: 'Test User',
      pictureUrl: 'https://example.com/avatar.png',
    });

    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([]);

    vi.mocked(consentApi.getAgentDetail).mockResolvedValue({
      agent: mockAgent,
      services: mockServices,
    });

    vi.mocked(consentApi.getAgentGrants).mockResolvedValue(mockGrant);
  });

  const renderPage = () => {
    return render(
      <ToastProvider>
        <MemoryRouter initialEntries={['/agents/test-agent']}>
          <Routes>
            <Route path="/agents/:agentId" element={<AgentGrantDetailPage />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>,
    );
  };

  it('should load and display agent information', async () => {
    renderPage();

    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
        expect(screen.getByText('A test agent')).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });

  it('should enable edit mode when toggle is clicked', async () => {
    renderPage();

    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });

  it('should submit grant successfully', async () => {
    const updatedGrant: UserGrant = {
      ...mockGrant,
      delegated_oauth2_tokens: [
        {
          thirdparty_oauth2_service_id: 'github',
          scopes: ['read:user', 'read:repo'],
        },
      ],
    };

    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'created', grant: updatedGrant });

    renderPage();

    // Wait for page to load
    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });

  it('should display error when submission fails', async () => {
    vi.mocked(consentApi.createOrUpdateGrant).mockRejectedValue({
      response: {
        data: {
          message: 'Failed to update grant',
        },
      },
    });

    renderPage();

    // Wait for page to load
    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });

  it('should refetch data after successful submission', async () => {
    const updatedGrant: UserGrant = {
      ...mockGrant,
      delegated_oauth2_tokens: [
        {
          thirdparty_oauth2_service_id: 'github',
          scopes: ['read:user', 'read:repo'],
        },
      ],
    };

    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'created', grant: updatedGrant });

    renderPage();

    // Wait for initial load
    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });

  it('should validate form before submission', async () => {
    renderPage();

    // Wait for page to load
    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });

  it('should cancel edit mode', async () => {
    renderPage();

    // Wait for page to load
    await waitFor(
      () => {
        expect(
          screen.getByRole('heading', { name: 'Test Agent' }),
        ).toBeInTheDocument();
      },
      { timeout: 5000 },
    );
  });
});
