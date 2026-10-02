export const connectionsCopy = {
  purpose: 'Manage the services you connect to your agents.',
  provider: 'Provider',
  scopes: 'Scopes',
  state: 'State',
  created: 'Created',
  actions: 'Actions',
  scopeCount: (count: number) => `${count} ${count === 1 ? 'scope' : 'scopes'}`,
  empty: 'You have no connections. Connect a service when an agent requests access.',
  readError: 'Your connections could not load. Try again.',
  stale: 'This connection may be out of date.',
  unavailable: 'Connection unavailable',
  refreshExplanation: 'Your access token expired. Refresh the connection to check whether access can continue.',
  refreshSuccess: 'Session token refreshed successfully.',
  refreshError: 'Failed to refresh session token. Please try again.',
  disconnectTitle: 'Disconnect service',
  disconnectDescription: (provider: string) => `Disconnect ${provider} from this broker?`,
  providerWarning: 'Disconnecting from this broker does not revoke provider-side tokens. You can revoke those tokens in the provider settings.',
  dependentAgents: 'These agents lose access through this connection:',
  noDependentAgents: 'No agents depend on this connection.',
  detailsError: 'Failed to load session details. Please try again.',
  disconnectSuccess: 'Session terminated successfully.',
  disconnectError: 'Failed to terminate session. Please try again.',
  callbackSuccess: 'Successfully connected to service. You can now delegate access to agents.',
  callbackFailure: 'Authorization failed. Please try again.',
} as const;

export function connectionCallbackError(code: string, description: string | null): string {
  const messages: Record<string, string> = {
    access_denied: 'You denied access to the service. No tokens were stored.',
    invalid_scope: 'The requested permissions are not available. Please contact support.',
    expired_token: 'Your session expired. Please try again.',
    callback_failed: description || connectionsCopy.callbackFailure,
    invalid_callback: 'Invalid response from service. Please try again.',
    invalid_state: 'Invalid request state. Please try again.',
    invalid_redirect_uri: 'Invalid redirect configuration. Please contact support.',
  };
  return Object.prototype.hasOwnProperty.call(messages, code) ? messages[code] : connectionsCopy.callbackFailure;
}
