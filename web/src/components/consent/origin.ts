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

export function getAgentOrigin(context: 'decision' | 'console', metadata?: CIMDMetadata | null): { label: string | null; localhost: boolean } {
  if (context === 'console') return { label: null, localhost: false };
  if (!metadata) return { label: accessCopy.registered, localhost: false };
  const host = hostname(metadata.verified_domain);
  return {
    label: host && !isLoopbackHost(metadata.verified_domain) ? accessCopy.verifiedDomain(host) : accessCopy.unverified,
    // Callback-localhost risk is independent of the metadata domain's trust.
    localhost: isLoopbackHost(metadata.verified_domain) || isLoopbackHost(metadata.client_id_url) || isLoopbackHost(metadata.redirect_uri),
  };
}
