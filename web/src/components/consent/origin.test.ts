import { describe, expect, it } from 'vitest';
import { accessCopy } from '@copy';
import type { CIMDMetadata } from '../../types/consent';
import { getAgentOrigin } from './origin';
const metadata: CIMDMetadata = { client_id_url: 'https://trusted.example/client', verified_domain: 'trusted.example', redirect_uri: 'https://trusted.example/callback', requested_scopes: [] };
describe('authorization origin labels', () => {
  it('shows a neutral registration fallback without inferring a registrant or redirect host', () => {
    const fallback = getAgentOrigin('decision', null);
    expect(fallback).toEqual({ label: accessCopy.registered, localhost: false, unverified: false });
    expect(fallback.label).not.toMatch(/administrator|verified domain/i);
    expect(getAgentOrigin('decision', { ...metadata, verified_domain: '' })).toEqual({ label: 'Unverified', localhost: false, unverified: true, redirectHost: 'trusted.example' });
  });
  it('keeps verified agent domain independent from a local callback and displays the callback host', () => {
    expect(getAgentOrigin('decision', { ...metadata, redirect_uri: 'http://localhost:3000/callback' })).toEqual({ label: 'Verified domain: trusted.example', localhost: true, unverified: false, redirectHost: 'localhost:3000' });
  });
  it.each(['localhost', 'localhost:8080', 'app.localhost', '127.0.0.1', '127.12.34.56:3000', '[::1]', '[::1]:8080'])('never trusts loopback agent metadata %s', (host) => {
    expect(getAgentOrigin('decision', { ...metadata, verified_domain: host })).toEqual({ label: 'Unverified', localhost: false, unverified: true, redirectHost: 'trusted.example' });
  });
  it('displays a different public callback host without a localhost warning', () => {
    expect(getAgentOrigin('decision', { ...metadata, redirect_uri: 'https://other.example/return' })).toEqual({ label: 'Verified domain: trusted.example', localhost: false, unverified: false, redirectHost: 'other.example' });
  });
  it('shows a nondefault callback port on the verified domain', () => {
    expect(getAgentOrigin('decision', { ...metadata, redirect_uri: 'https://trusted.example:8443/callback' }).redirectHost).toBe('trusted.example:8443');
  });
  it('never invents a redirect host from malformed callback metadata', () => {
    expect(getAgentOrigin('decision', { ...metadata, redirect_uri: 'not a url' })).toEqual({ label: 'Verified domain: trusted.example', localhost: false, unverified: false });
  });
  it('does not classify similarly named public domains as loopback', () => {
    expect(getAgentOrigin('decision', { ...metadata, verified_domain: 'notlocalhost.example' })).toEqual({ label: 'Verified domain: notlocalhost.example', localhost: false, unverified: false, redirectHost: 'trusted.example' });
  });
  it('never adds a verification claim outside an authorization session', () => {
    expect(getAgentOrigin('console', metadata)).toEqual({ label: null, localhost: false, unverified: false });
  });
});
