export { consentCopy } from './consent';
export { approvalCopy } from './approvals';
export { approvalQueueCopy } from './approvalQueue';
export { connectionsCopy, connectionCallbackError } from './connections';
export { delegationsCopy } from './delegations';
export { commandCopy } from './command';
export { settingsCopy } from './settings';

export const commonCopy = {
  brand: 'Agentic Identity Broker',
  loading: 'Loading…',
  retry: 'Try again',
  cancel: 'Cancel',
  close: 'Close',
  save: 'Save changes',
  showMore: 'More',
  showLess: 'Less',
  notifications: 'Notifications',
  errorTitle: 'Something went wrong',
  errorMessage: 'Try again. Your access has not changed.',
  authenticationRequired: 'Sign in through your organization to continue.',
  accountLoadError: 'Try again to load your account.',
  notFoundCode: '404',
  pageNotFound: 'Page not found',
  pageNotFoundDescription: 'Check the address or return to your agents.',
  returnToAgents: 'Go to Agents',
} as const;

export const navigationCopy = {
  agents: 'Agents',
  connections: 'Connections',
  approvals: 'Approvals',
  settings: 'Settings',
  mainNavigation: 'Main navigation',
  skipToContent: 'Skip to content',
  openNavigation: 'Open navigation',
  closeNavigation: 'Close navigation',
  collapseSidebar: 'Collapse sidebar',
  expandSidebar: 'Expand sidebar',
  search: 'Search',
  searchMacShortcut: '⌘K',
  searchControlShortcut: 'Ctrl K',
  userMenu: 'User menu',
  documentation: 'Documentation',
  documentationUrl: 'https://agenticidentitybroker.dev/docs/introduction',
  navigationDescription: 'Choose a console page.',
  pendingLoading: 'Loading pending approvals.',
  pendingCount: (count: number) => `${count} pending ${count === 1 ? 'approval' : 'approvals'}`,
  pendingUnavailable: 'Pending approvals are unavailable.',
  pendingStale: 'Pending approvals may be out of date.',
} as const;

export const themeCopy = {
  light: 'Light',
  dark: 'Dark',
  system: 'System',
  label: 'Appearance',
} as const;

export const accessCopy = {
  verifiedDomain: (host: string) => `Verified domain: ${host}`,
  registered: 'Registered agent',
  unverified: 'Unverified',
  riskNotRated: 'Risk not rated',
  untilRevoked: 'Until revoked',
  thirtyDays: '30 days',
  customDate: 'Custom date',
  allow: 'Allow',
  deny: 'Deny',
  approveOnce: 'Approve once',
  approveAndRemember: 'Approve and remember',
  revokeAllAccess: 'Revoke all access',
  revoke: 'Revoke',
  revokeTitle: 'Revoke access',
  revokeDescription: (agentName: string) =>
    `Revoke ${agentName}'s delegated access through this broker? The agent needs your permission for new access. Your own connections to OAuth2 services remain active.`,
  connected: 'Connected',
  needsReauthentication: 'Needs sign-in',
  expired: 'Expired',
  noConnection: 'No connection',
  connect: 'Connect',
  reconnect: 'Reconnect',
  refresh: 'Refresh',
  disconnect: 'Disconnect',
} as const;

export const collectionCopy = {
  search: 'Search',
  sort: 'Sort',
  filter: 'Filter',
  grid: 'Grid view',
  list: 'List view',
  clearSearch: 'Clear search',
  noResults: 'No results',
  name: 'Name',
  recentlyChanged: 'Recently changed',
  expiringSoonest: 'Expiring soonest',
  all: 'All',
  clearFilter: 'Clear filter',
} as const;
