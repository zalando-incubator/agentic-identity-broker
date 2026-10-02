import { describe, expect, it } from 'vitest';
import { getAgentOrigin } from './origin';
import type { CIMDMetadata } from '../../types/consent';
const metadata: CIMDMetadata = { client_id_url: 'https://trusted.example/client', verified_domain: 'trusted.example', redirect_uri: 'https://trusted.example/callback', requested_scopes: [] };
describe('authorization origin labels', () => {
  it('distinguishes an administrator registration from unverified CIMD', () => {
    expect(getAgentOrigin('decision', null)).toEqual({ label: 'Registered by your administrator', localhost: false });
    expect(getAgentOrigin('decision', { ...metadata, verified_domain: '' }).label).toBe('Unverified');
  });
  it('keeps domain verification independent from a localhost redirect warning', () => {
    expect(getAgentOrigin('decision', { ...metadata, redirect_uri: 'http://localhost:3000/callback' })).toEqual({ label: 'Verified domain: trusted.example', localhost: true });
  });
  it.each(['localhost', 'localhost:8080', 'app.localhost', '127.0.0.1', '127.12.34.56:3000', '[::1]', '[::1]:8080'])('never trusts loopback metadata %s', (host) => {
    expect(getAgentOrigin('decision', { ...metadata, verified_domain: host })).toEqual({ label: 'Unverified', localhost: true });
  });
  it('does not classify similarly named public domains as loopback', () => {
    expect(getAgentOrigin('decision', { ...metadata, verified_domain: 'notlocalhost.example' })).toEqual({ label: 'Verified domain: notlocalhost.example', localhost: false });
  });
  it('never adds a verification claim outside an authorization session', () => {
    expect(getAgentOrigin('console', metadata)).toEqual({ label: null, localhost: false });
  });
});
