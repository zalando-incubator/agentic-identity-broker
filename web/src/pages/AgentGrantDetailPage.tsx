/**
 * AgentGrantDetailPage - Detailed view for managing grants to a specific agent.
 *
 * Shows:
 * - Agent details (logo, name, description)
 * - Links to governance and documentation
 * - List of available services with scopes
 * - Existing grants and their status
 * - Interactive grant editing (Phase 5)
 *
 * Features:
 * - Toast notifications for success/error feedback
 * - Page transition animations
 * - Smooth scroll to errors on validation failure
 */

import React, { useState, useCallback, useMemo } from 'react';
import { useParams, useLocation, useNavigate } from 'react-router-dom';
import { AppLayout } from '@components/layout/AppLayout';
import { PageTransition } from '@components/ui/PageTransition';
import { Skeleton } from '@components/ui/Skeleton';
import { InlineError } from '@components/ui/InlineError';
import { Button } from '@components/ui/Button';
import { useToast } from '@components/ui/Toast';
import { Breadcrumb } from '@design-system/components/navigation/Breadcrumb';
import { Alert } from '@design-system/components/feedback/Alert';
import { Card } from '@design-system/components/data-display/Card';
import { ServiceCard } from '@components/consent/ServiceCard';
import { RevokeGrantButton } from '@components/consent/RevokeGrantButton';
import { CIMDSection } from '@components/consent/CIMDSection';
import { GrantValidityControl } from '@components/consent/GrantValidityControl';
import { PermissionSetsList } from '@components/consent/PermissionSetsList';
import { useAgentGrants, useToggleGrant, useUpdateValidity } from '@hooks';
import {
  clearPendingConsentSelections,
  loadPendingConsentSelections,
  saveConsentSelections,
} from '@services/storage/session';
import { validateGrantRequest, isSafeRedirectUrl } from '../utils/validation';
import { scrollToError } from '../utils/scrollToError';

/**
 * AgentGrantDetailPage displays detailed agent information and service grants.
 * Allows users to view and edit which services and scopes are granted to the agent.
 */
export function AgentGrantDetailPage() {
  const { agentId } = useParams<{ agentId: string }>();
  const { showToast } = useToast();
  const location = useLocation();
  const navigate = useNavigate();

  const searchParams = new URLSearchParams(location.search);
  const sessionToken = searchParams.get('session_token') || undefined;

  const restoredSelections = useMemo(() => {
    const callbackParams = new URLSearchParams(location.search);
    return callbackParams.get('success') === 'true'
      ? loadPendingConsentSelections()
      : undefined;
  }, [location.search]);

  const resolvedAgentId = agentId ?? '';

  // Memoize options to keep a stable object reference across renders.
  // Without this, { sessionToken } creates a new object every render, causing
  // useCallback in useAgentGrants to recreate fetchData, which triggers
  // useEffect on every render, causing an infinite loading loop.
  const agentGrantOptions = useMemo(
    () => (sessionToken ? { sessionToken } : undefined),
    [sessionToken],
  );

  // Fetch agent data and grants
  const {
    agent,
    services,
    cimdMeta,
    grants,
    loading,
    error,
    sessionExpired,
    refetch,
  } = useAgentGrants(resolvedAgentId, agentGrantOptions);

  // Grant toggle hook for permission sets
  const {
    isSubmitting,
    error: submitError,
    submit,
    clearError,
  } = useToggleGrant(resolvedAgentId);

  const hasExistingGrant = grants !== null;

  // Validity hook (initialized from grant if it exists)
  const {
    validityState,
    setValidityState,
    getValidUntil,
    validate: validateValidity,
  } = useUpdateValidity(grants);

  // Track per-PS per-service inclusion from PermissionSetsList (FR-008, FR-011)
  const [perPsIncludedServiceIds, setPerPsIncludedServiceIds] = useState<
    Record<string, string[]>
  >({});

  // Validation errors
  const [validationErrors, setValidationErrors] = useState<string[]>([]);

  // Handle grants change
  // Handle validity change
  const handleValidityChange = (state: typeof validityState) => {
    setValidityState(state);
    setValidationErrors([]);
  };

  // Validate form
  const validateForm = (grantedPS: Record<string, string[]>): boolean => {
    const errors: string[] = [];

    // Require at least one PS for agents that declare permission sets (FR-014).
    // An empty granted_permission_sets list is never valid for PS-using agents,
    // even when all PSes are optional.
    const requireAtLeastOneService = (agent?.permission_sets?.length ?? 0) > 0;

    // Validate grant request
    const grantErrors = validateGrantRequest({
      grantedPermissionSets: grantedPS,
      validUntil: getValidUntil(),
      requireAtLeastOneService,
    });
    errors.push(...grantErrors);

    // Validate validity
    const validityError = validateValidity();
    if (validityError) {
      errors.push(validityError);
    }

    setValidationErrors(errors);

    // Scroll to first error if validation failed
    if (errors.length > 0) {
      setTimeout(() => {
        scrollToError();
      }, 100);
    }

    return errors.length === 0;
  };

  // Build structured granted_permission_sets from current selection.
  // Uses perPsIncludedServiceIds from PermissionSetsList for per-service inclusion (FR-011).
  const buildGrantedPermissionSets = (): Record<string, string[]> => {
    // perPsIncludedServiceIds is populated by PermissionSetsList on mount (including initial
    // grant hydration), so it is the single authoritative source for submission.
    if (Object.keys(perPsIncludedServiceIds).length > 0) {
      return { ...perPsIncludedServiceIds };
    }

    // Fallback before PermissionSetsList has mounted and emitted:
    // include mandatory PSes with service_scopes intersected with agent's SR (FR-011)
    const srServiceIds = new Set(
      agent?.service_requirements?.map((sr) => sr.service_id) ?? [],
    );
    const result: Record<string, string[]> = {};
    agent?.permission_sets
      ?.filter((ps) => ps.requirement_type === 'mandatory')
      .forEach((ps) => {
        const serviceIds = ps.permission_set.service_scopes
          .filter(
            (ss) => srServiceIds.size === 0 || srServiceIds.has(ss.service_id),
          )
          .map((ss) => ss.service_id);
        result[ps.permission_set.id] = serviceIds;
      });
    return result;
  };

  // Handle form submission
  const handleSubmit = async () => {
    const grantedPS = buildGrantedPermissionSets();

    clearError();
    setValidationErrors([]);

    if (!validateForm(grantedPS)) {
      return;
    }

    const validUntil = getValidUntil();
    const submitOptions = sessionToken ? { sessionToken } : undefined;

    let grant: Awaited<ReturnType<typeof submit>>;
    try {
      grant = await submit(validUntil, submitOptions, grantedPS);
    } catch (err) {
      showToast(
        err instanceof Error ? err.message : 'Failed to update grant',
        'error',
      );
      return;
    }

    if (!grant) {
      showToast('An unexpected error occurred. Please try again.', 'error');
      return;
    }

    if (grant.kind === 'redirect') {
      if (!isSafeRedirectUrl(grant.redirectUrl)) {
        showToast('Invalid redirect URL', 'error');
        return;
      }
      window.location.href = grant.redirectUrl;
      return;
    }

    if (grant.kind === 'created') {
      await refetch();
      showToast('Grant updated successfully!', 'success');
    }
    // 'noContent' — grant revoked, no further action
  };

  const handleServiceLogin = useCallback(
    (serviceId: string) => {
      const currentUrl = new URL(window.location.href);
      currentUrl.searchParams.delete('consent_state');
      currentUrl.searchParams.delete('consent_state_id');

      if (Object.keys(perPsIncludedServiceIds).length === 0) {
        clearPendingConsentSelections();
        window.location.href = `/api/third-party/${serviceId}/oauth2/authorize?redirect_uri=${encodeURIComponent(currentUrl.toString())}`;
        return;
      }

      const stateID = saveConsentSelections(perPsIncludedServiceIds);
      if (!stateID) {
        showToast('Unable to preserve selections. Please try again.', 'error');
        return;
      }

      const form = document.createElement('form');
      form.method = 'post';
      form.action = `/api/third-party/${serviceId}/oauth2/authorize`;
      form.style.display = 'none';
      for (const [name, value] of Object.entries({
        redirect_uri: currentUrl.toString(),
        consent_state_id: stateID,
      })) {
        const input = document.createElement('input');
        input.name = name;
        input.value = value;
        form.appendChild(input);
      }
      document.body.appendChild(form);
      form.submit();
    },
    [perPsIncludedServiceIds, showToast],
  );

  // Handle full grant deletion (Revoke All Access button)
  const handleGrantRevoked = useCallback(() => {
    navigate('/');
    showToast('All access for this agent has been revoked.', 'success');
  }, [navigate, showToast]);

  // Handle service login (service without active session)
  const handleServiceConnect = useCallback(
    (serviceId: string) => {
      handleServiceLogin(serviceId);
    },
    [handleServiceLogin],
  );

  // Compute dynamic service connections based on selected PSes
  // When permission sets exist: union of (1) all services from mandatory PSes + (2) selected optional PS services
  // When no permission sets: null (show all services — backward compatible)
  const dynamicServiceIds = useMemo((): Set<string> | null => {
    if (!agent?.permission_sets || agent.permission_sets.length === 0) {
      return null; // No PS filtering — show all services
    }

    // Use perPsIncludedServiceIds as the authoritative source when available (FR-009, FR-010)
    // This reflects per-service toggle state from PermissionSetsList.
    if (Object.keys(perPsIncludedServiceIds).length > 0) {
      const serviceIds = new Set<string>();
      Object.values(perPsIncludedServiceIds).forEach((ids) =>
        ids.forEach((id) => serviceIds.add(id)),
      );
      return serviceIds;
    }

    // Fallback before PermissionSetsList has reported state:
    // include services from mandatory PSes intersected with agent's SR (FR-009)
    const srServiceIds = new Set(
      agent.service_requirements?.map((sr) => sr.service_id) ?? [],
    );
    const serviceIds = new Set<string>();
    agent.permission_sets
      .filter((ps) => ps.requirement_type === 'mandatory')
      .forEach((ps) => {
        ps.permission_set.service_scopes.forEach((ss) => {
          if (srServiceIds.size === 0 || srServiceIds.has(ss.service_id)) {
            serviceIds.add(ss.service_id);
          }
        });
      });
    return serviceIds;
  }, [agent, perPsIncludedServiceIds]);

  // Filter services: show only those without active sessions
  // When dynamicServiceIds is null (no PSes), show ALL services without active sessions
  // When dynamicServiceIds is set, only show services in the dynamic set
  const servicesWithoutActiveSessions = useMemo(() => {
    return services.filter(
      (service) =>
        (dynamicServiceIds === null ||
          dynamicServiceIds.has(service.serviceId)) &&
        !agent?.active_session_service_ids?.includes(service.serviceId),
    );
  }, [services, dynamicServiceIds, agent]);

  // Compute effective requirement types for service connections badges.
  // A service is effectively mandatory if SR.requirement_type=mandatory OR any currently-active PS
  // has it as ServiceScope.requirement_type=mandatory (FR-009, FR-014).
  const effectiveRequirementTypes = useMemo(() => {
    const result = new Map<string, 'mandatory' | 'optional'>();
    for (const service of services) {
      result.set(service.serviceId, service.requirementType ?? 'optional');
    }
    // Use perPsIncludedServiceIds as authoritative source; fall back to mandatory PS IDs
    // before PermissionSetsList has mounted and emitted.
    const activePsIds =
      Object.keys(perPsIncludedServiceIds).length > 0
        ? Object.keys(perPsIncludedServiceIds)
        : (agent?.permission_sets
            ?.filter((p) => p.requirement_type === 'mandatory')
            .map((p) => p.permission_set.id) ?? []);
    for (const psId of activePsIds) {
      const psEntry = agent?.permission_sets?.find(
        (p) => p.permission_set.id === psId,
      );
      if (!psEntry) continue;
      for (const ss of psEntry.permission_set.service_scopes) {
        if (ss.requirement_type === 'mandatory') {
          result.set(ss.service_id, 'mandatory');
        }
      }
    }
    return result;
  }, [services, agent, perPsIncludedServiceIds]);

  // FR-020: Approve button disabled until all displayed dynamic services have active sessions
  const isApproveDisabled = useMemo(() => {
    if (dynamicServiceIds === null) {
      return false; // No permission sets on agent — don't gate on services
    }
    if (dynamicServiceIds.size === 0) {
      return false; // No services in selection (e.g., all optional PSes unselected with no mandatory PSes covering services)
    }
    // Check every service in the dynamic set has an active session
    const activeSet = new Set(agent?.active_session_service_ids || []);
    for (const svcId of dynamicServiceIds) {
      if (!activeSet.has(svcId)) {
        return true;
      }
    }
    return false;
  }, [agent, dynamicServiceIds]);

  // Validate agentId parameter (after hooks to keep hook order stable)
  if (!agentId) {
    return (
      <AppLayout>
        <div className="space-y-6">
          <InlineError
            error="Invalid agent ID in URL"
            onRetry={() => window.history.back()}
          />
        </div>
      </AppLayout>
    );
  }

  // Loading state
  if (loading) {
    return (
      <AppLayout>
        <div className="space-y-6">
          {/* Breadcrumb skeleton */}
          <Breadcrumb
            items={[
              { label: 'Delegations', href: '/' },
              { label: 'Loading...' },
            ]}
          />

          {/* Agent header skeleton */}
          <Card padding="default">
            <div className="flex items-start gap-4">
              <Skeleton width="80px" height="80px" className="rounded-lg" />
              <div className="flex-1 space-y-3">
                <Skeleton width="60%" height="2rem" />
                <Skeleton width="80%" height="1rem" />
                <Skeleton width="40%" height="1rem" />
              </div>
            </div>
          </Card>

          {/* Services skeleton */}
          <div className="space-y-4">
            <Skeleton width="150px" height="1.5rem" />
            <div className="space-y-4">
              <Skeleton width="100%" height="200px" className="rounded-lg" />
              <Skeleton width="100%" height="200px" className="rounded-lg" />
            </div>
          </div>
        </div>
      </AppLayout>
    );
  }

  // Session expired — token cannot be retried; user must restart the authorization flow
  if (sessionExpired) {
    return (
      <AppLayout>
        <div className="space-y-6">
          <Breadcrumb
            items={[
              { label: 'Delegations', href: '/' },
              { label: 'Agent Details' },
            ]}
          />
          <InlineError
            error="Your authorization session has expired. Please go back and restart the authorization flow."
            onRetry={() => window.history.back()}
            retryLabel="Go back"
          />
        </div>
      </AppLayout>
    );
  }

  // Error state
  if (error) {
    return (
      <AppLayout>
        <div className="space-y-6">
          {/* Breadcrumb */}
          <Breadcrumb
            items={[
              { label: 'Delegations', href: '/' },
              { label: 'Agent Details' },
            ]}
          />

          <InlineError error={error} onRetry={refetch} />
        </div>
      </AppLayout>
    );
  }

  // No agent data (should not happen if no error)
  if (!agent) {
    return (
      <AppLayout>
        <div className="space-y-6">
          <InlineError error="Agent data not available" onRetry={refetch} />
        </div>
      </AppLayout>
    );
  }

  return (
    <AppLayout>
      <PageTransition>
        <div className="space-y-6">
          {/* Breadcrumb */}
          <Breadcrumb
            items={[
              { label: 'Delegations', href: '/' },
              { label: agent.displayName },
            ]}
          />

          {/* Agent header card */}
          <Card padding="default">
            <div className="flex items-start gap-6">
              {/* Agent logo */}
              <div className="flex-shrink-0">
                {agent.logoUrl ? (
                  <img
                    src={agent.logoUrl}
                    alt={`${agent.displayName} logo`}
                    className="w-20 h-20 rounded-lg object-cover"
                    referrerPolicy="no-referrer"
                  />
                ) : (
                  <div className="w-20 h-20 bg-gradient-to-br from-trust to-trust-hover rounded-lg flex items-center justify-center">
                    <span className="text-white text-2xl font-semibold">
                      {agent.displayName.charAt(0).toUpperCase()}
                    </span>
                  </div>
                )}
              </div>

              {/* Agent info */}
              <div className="flex-1 min-w-0">
                <div className="flex items-start justify-between gap-4">
                  <div className="flex-1 min-w-0">
                    <h1
                      className="text-2xl font-display font-bold text-trust-deep"
                      data-testid="agent-name-heading"
                    >
                      {agent.displayName}
                    </h1>
                    <p className="mt-2 text-neutral-600">{agent.description}</p>
                  </div>
                </div>

                {/* Agent links */}
                <div className="mt-4 flex flex-wrap items-center gap-4">
                  {agent.governanceUrl && (
                    <a
                      href={agent.governanceUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-2 text-sm font-medium text-trust-hover hover:text-trust focus:outline-none focus:ring-2 focus:ring-trust focus:ring-offset-2 rounded"
                    >
                      <svg
                        className="w-4 h-4"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
                        />
                      </svg>
                      Governance
                    </a>
                  )}
                  {agent.userDocumentationUrl && (
                    <a
                      href={agent.userDocumentationUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-2 text-sm font-medium text-trust-hover hover:text-trust focus:outline-none focus:ring-2 focus:ring-trust focus:ring-offset-2 rounded"
                    >
                      <svg
                        className="w-4 h-4"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"
                        />
                      </svg>
                      Documentation
                    </a>
                  )}
                  {agent.agentInterfaceUrl && (
                    <a
                      href={agent.agentInterfaceUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-2 text-sm font-medium text-trust-hover hover:text-trust focus:outline-none focus:ring-2 focus:ring-trust focus:ring-offset-2 rounded"
                    >
                      <svg
                        className="w-4 h-4"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
                        />
                      </svg>
                      Agent Interface
                    </a>
                  )}
                </div>
              </div>
            </div>
          </Card>

          {/* CIMD metadata section — shown only for URL-based (CIMD) agents */}
          {cimdMeta && (
            <CIMDSection
              cimdMeta={cimdMeta}
              agentDisplayName={agent.displayName}
              agentLogoUrl={agent.logoUrl}
              services={services}
            />
          )}

          {/* Validation errors */}
          {validationErrors.length > 0 && (
            <Alert
              variant="warning"
              title="Validation Error"
              data-error="true"
              role="alert"
              aria-live="assertive"
            >
              <ul className="list-disc list-inside">
                {validationErrors.map((error, index) => (
                  <li key={index}>{error}</li>
                ))}
              </ul>
            </Alert>
          )}

          {/* Submit error */}
          {submitError && (
            <InlineError error={submitError} onRetry={handleSubmit} />
          )}

          {/* End date section */}
          <div>
            <h2 className="text-xl font-semibold text-trust-deep mb-4">
              End Date
            </h2>
            <GrantValidityControl
              value={validityState}
              onChange={handleValidityChange}
            />
          </div>

          {/* Permission Sets section */}
          {agent?.permission_sets && agent.permission_sets.length > 0 && (
            <PermissionSetsList
              permissionSets={agent.permission_sets}
              availableServices={services.map((s) => ({
                id: s.serviceId,
                display_name:
                  s.kind === 'requirement'
                    ? s.serviceName
                    : (s.displayName ?? s.serviceId),
              }))}
              serviceRequirements={agent.service_requirements}
              initialGrantedPermissionSets={
                restoredSelections ?? grants?.granted_permission_sets ?? {}
              }
              onSelectionChange={(_optionalIds, perPsIncluded) => {
                setPerPsIncludedServiceIds(perPsIncluded);
              }}
            />
          )}

          {/* Services section - Services without active sessions */}
          {servicesWithoutActiveSessions.length > 0 && (
            <div className="space-y-4">
              <div>
                <h2 className="text-xl font-semibold text-trust-deep">
                  Connect Your Accounts
                  <span className="ml-2 text-sm font-normal text-neutral-500">
                    ({servicesWithoutActiveSessions.length})
                  </span>
                </h2>
                <p className="mt-1 text-sm text-neutral-600">
                  {agent.displayName} needs access to the following services.
                  Click <strong>Login</strong> to authorize each one before{' '}
                  {hasExistingGrant ? 'saving.' : 'approving.'}
                </p>
              </div>

              {/* Single stacked card matching the Agent Permissions layout */}
              <Card padding="none" border="subtle" hover="none">
                {servicesWithoutActiveSessions.map((service, index) => (
                  <React.Fragment key={service.serviceId}>
                    {index > 0 && <hr className="border-neutral-200" />}
                    <ServiceCard
                      service={{
                        ...service,
                        requirementType:
                          effectiveRequirementTypes.get(service.serviceId) ??
                          service.requirementType ??
                          'optional',
                      }}
                      grants={[]}
                      isDelegated={false}
                      onDelegate={() => handleServiceConnect(service.serviceId)}
                      onRevoke={() => {}}
                      inStack
                      isFirst={index === 0}
                      isLast={
                        index === servicesWithoutActiveSessions.length - 1
                      }
                    />
                  </React.Fragment>
                ))}
              </Card>
            </div>
          )}

          {/* Action buttons */}
          <div className="flex items-center justify-between gap-3 pt-4">
            {/* Revoke All Access — only visible when user has an active grant */}
            {grants !== null &&
              grants.granted_permission_sets &&
              Object.keys(grants.granted_permission_sets).length > 0 && (
                <RevokeGrantButton
                  agentId={resolvedAgentId}
                  agentName={agent.displayName}
                  onRevoked={handleGrantRevoked}
                />
              )}

            <Button
              variant="primary"
              onClick={handleSubmit}
              isLoading={isSubmitting}
              disabled={isApproveDisabled}
              title={
                isApproveDisabled
                  ? hasExistingGrant
                    ? 'Connect all required services before saving'
                    : 'Connect all required services before approving'
                  : hasExistingGrant
                    ? 'Save changes to these permissions'
                    : 'Approve and delegate these permissions'
              }
            >
              {hasExistingGrant ? 'Save' : 'Approve & Delegate'}
            </Button>
          </div>
        </div>
      </PageTransition>
    </AppLayout>
  );
}

export default AgentGrantDetailPage;
