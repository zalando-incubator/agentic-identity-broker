import { accessCopy } from '@copy';
import type { CIMDMetadata } from '../../types/consent';

function hostname(value: string): string | undefined {
  try {
    const url = new URL(value.includes('://') ? value : `https://${value}`);
    return url.hostname.toLowerCase().replace(/\.$/, '');
  } catch {
    return undefined;
  }
}

export function isLoopbackHost(value: string): boolean {
  const host = hostname(value);
  return host !== undefined && (host === 'localhost' || host.endsWith('.localhost')
    || host === '[::1]' || /^127(?:\.\d{1,3}){3}$/.test(host));
}

export interface AgentOrigin {
  label: string | null;
  localhost: boolean;
  unverified: boolean;
  redirectHost?: string;
}

export function getAgentOrigin(context: 'decision' | 'console', metadata?: CIMDMetadata | null): AgentOrigin {
  if (context === 'console') return { label: null, localhost: false, unverified: false };
  if (!metadata) return { label: accessCopy.registered, localhost: false, unverified: false };
  const host = hostname(metadata.verified_domain);
  const verified = Boolean(host && !isLoopbackHost(metadata.verified_domain));
  let redirectHost: string | undefined;
  try {
    const redirect = new URL(metadata.redirect_uri);
    if ((redirect.protocol === 'https:' || redirect.protocol === 'http:') && redirect.host.toLowerCase() !== host) redirectHost = redirect.host;
  } catch {
    // Only validated redirect metadata can provide a host to display.
  }
  return {
    label: verified ? accessCopy.verifiedDomain(host!) : accessCopy.unverified,
    localhost: isLoopbackHost(metadata.redirect_uri),
    unverified: !verified,
    ...(redirectHost && { redirectHost }),
  };
}
