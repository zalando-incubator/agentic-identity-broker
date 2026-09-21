/**
 * Tests for validation utilities.
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import {
  validateGrantRequest,
  validateServiceId,
  validateScopes,
  validateExpirationDate,
  formatValidationErrors,
  isSafeRedirectUrl,
} from './validation';

describe('isSafeRedirectUrl', () => {
  const originalLocation = window.location;

  beforeEach(() => {
    // Mock window.location for testing
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: {
        ...originalLocation,
        protocol: 'https:',
        hostname: 'example.com',
        port: '443',
        origin: 'https://example.com',
      } as Location,
    });
  });

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation,
    });
  });

  describe('same-origin URLs', () => {
    it('should accept same-origin absolute URLs', () => {
      expect(isSafeRedirectUrl('https://example.com/consent')).toBe(true);
      expect(isSafeRedirectUrl('https://example.com/sessions')).toBe(true);
      expect(isSafeRedirectUrl('https://example.com:443/consent')).toBe(true);
    });

    it('should accept relative URLs', () => {
      expect(isSafeRedirectUrl('/consent')).toBe(true);
      expect(isSafeRedirectUrl('/sessions')).toBe(true);
      expect(isSafeRedirectUrl('consent')).toBe(true);
      expect(isSafeRedirectUrl('./consent')).toBe(true);
      expect(isSafeRedirectUrl('../consent')).toBe(true);
    });
  });

  describe('cross-origin URLs', () => {
    it('should reject different domain', () => {
      expect(isSafeRedirectUrl('https://evil.com/phishing')).toBe(false);
      expect(isSafeRedirectUrl('https://attacker.example.com/phishing')).toBe(
        false,
      );
    });

    it('should reject different protocol', () => {
      expect(isSafeRedirectUrl('http://example.com/consent')).toBe(false);
      expect(isSafeRedirectUrl('ftp://example.com/consent')).toBe(false);
    });

    it('should reject different port', () => {
      expect(isSafeRedirectUrl('https://example.com:8080/consent')).toBe(false);
      expect(isSafeRedirectUrl('https://example.com:80/consent')).toBe(false);
    });

    it('should reject protocol-relative URLs', () => {
      expect(isSafeRedirectUrl('//evil.com/phishing')).toBe(false);
      expect(isSafeRedirectUrl('//example.com/consent')).toBe(false);
    });
  });

  describe('edge cases', () => {
    it('should reject empty or null URLs', () => {
      expect(isSafeRedirectUrl('')).toBe(false);
    });

    it('should reject dangerous protocols', () => {
      expect(isSafeRedirectUrl('javascript:alert(1)')).toBe(false);
      expect(
        isSafeRedirectUrl('data:text/html,<script>alert(1)</script>'),
      ).toBe(false);
      expect(isSafeRedirectUrl('vbscript:msgbox(1)')).toBe(false);
      expect(isSafeRedirectUrl('file:///etc/passwd')).toBe(false);
      expect(isSafeRedirectUrl('about:blank')).toBe(false);
      expect(isSafeRedirectUrl('blob:https://example.com/uuid')).toBe(false);
    });

    it('should reject dangerous protocols regardless of case', () => {
      expect(isSafeRedirectUrl('JavaScript:alert(1)')).toBe(false);
      expect(
        isSafeRedirectUrl('DATA:text/html,<script>alert(1)</script>'),
      ).toBe(false);
      expect(isSafeRedirectUrl('VBScript:msgbox(1)')).toBe(false);
    });

    it('should accept relative URLs with :// in query params', () => {
      // This is a legitimate relative URL with :// in query string
      expect(isSafeRedirectUrl('/consent?redirect=http://example.com')).toBe(
        true,
      );
    });

    it('should reject unknown protocols', () => {
      expect(isSafeRedirectUrl('ftp://example.com/file')).toBe(false);
    });
  });

  describe('HTTP context', () => {
    beforeEach(() => {
      Object.defineProperty(window, 'location', {
        configurable: true,
        value: {
          ...originalLocation,
          protocol: 'http:',
          hostname: 'localhost',
          port: '8000',
          origin: 'http://localhost:8000',
        } as Location,
      });
    });

    it('should accept same-origin HTTP URLs', () => {
      expect(isSafeRedirectUrl('http://localhost:8000/consent')).toBe(true);
      expect(isSafeRedirectUrl('/consent')).toBe(true);
    });

    it('should reject HTTPS URLs when origin is HTTP', () => {
      expect(isSafeRedirectUrl('https://localhost:8000/consent')).toBe(false);
    });

    it('should reject different port', () => {
      expect(isSafeRedirectUrl('http://localhost:3000/consent')).toBe(false);
    });
  });
});

describe('validateGrantRequest', () => {
  it('should accept valid grant request', () => {
    const request = {
      grantedPermissionSets: { 'ps-id-1': ['svc-1'], 'ps-id-2': ['svc-2'] },
    };

    const errors = validateGrantRequest(request);
    expect(errors).toEqual([]);
  });

  it('should require grantedPermissionSets field', () => {
    const request = {} as { grantedPermissionSets: Record<string, string[]> };
    const errors = validateGrantRequest(request);
    expect(errors).toContain('grantedPermissionSets field is required');
  });

  it('should require at least one permission set when mandatory requirements exist', () => {
    const request = {
      grantedPermissionSets: {},
      requireAtLeastOneService: true,
    };

    const errors = validateGrantRequest(request);
    expect(errors).toContain('Please select at least one permission set');
  });

  it('should not require a permission set when only optional requirements exist', () => {
    const request = {
      grantedPermissionSets: {},
      requireAtLeastOneService: false,
    };

    const errors = validateGrantRequest(request);
    expect(errors).not.toContain('Please select at least one permission set');
    expect(errors).toHaveLength(0);
  });

  it('should require at least one permission set by default (no requireAtLeastOneService flag)', () => {
    const request = {
      grantedPermissionSets: {},
    };

    const errors = validateGrantRequest(request);
    expect(errors).toContain('Please select at least one permission set');
  });

  it('should require at least one service per permission set', () => {
    const request = {
      grantedPermissionSets: { 'ps-id-1': [] },
    };

    const errors = validateGrantRequest(request);
    expect(errors.length).toBeGreaterThan(0);
  });
});

describe('validateServiceId', () => {
  it('should accept valid service IDs', () => {
    expect(validateServiceId('service-1')).toBe(true);
    expect(validateServiceId('my_service')).toBe(true);
    expect(validateServiceId('service.123')).toBe(true);
  });

  it('should reject invalid service IDs', () => {
    expect(validateServiceId('')).toBe(false);
    expect(validateServiceId('service with spaces')).toBe(false);
    expect(validateServiceId('service@special')).toBe(false);
  });
});

describe('validateScopes', () => {
  it('should accept valid scopes', () => {
    expect(validateScopes(['read', 'write'])).toBe(true);
    expect(validateScopes(['admin'])).toBe(true);
  });

  it('should reject empty or invalid scopes', () => {
    expect(validateScopes([])).toBe(true); // Empty array is technically valid
    expect(validateScopes([''])).toBe(false);
    expect(validateScopes(['  '])).toBe(false);
  });
});

describe('validateExpirationDate', () => {
  it('should accept future dates', () => {
    const futureDate = new Date();
    futureDate.setDate(futureDate.getDate() + 1);
    expect(validateExpirationDate(futureDate)).toBeNull();
  });

  it('should reject past dates', () => {
    const pastDate = new Date();
    pastDate.setDate(pastDate.getDate() - 1);
    expect(validateExpirationDate(pastDate)).toBe(
      'End date must be in the future',
    );
  });

  it('should reject dates more than 10 years in future', () => {
    const farFutureDate = new Date();
    farFutureDate.setFullYear(farFutureDate.getFullYear() + 11);
    expect(validateExpirationDate(farFutureDate)).toBe(
      'End date cannot be more than 10 years in the future',
    );
  });
});

describe('formatValidationErrors', () => {
  it('should return empty string for no errors', () => {
    expect(formatValidationErrors([])).toBe('');
  });

  it('should return single error as-is', () => {
    expect(formatValidationErrors(['Error 1'])).toBe('Error 1');
  });

  it('should format multiple errors as numbered list', () => {
    const errors = ['Error 1', 'Error 2', 'Error 3'];
    const formatted = formatValidationErrors(errors);
    expect(formatted).toBe('1. Error 1\n2. Error 2\n3. Error 3');
  });
});
