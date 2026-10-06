/**
 * Validation utilities for grant requests.
 *
 * Provides validation functions for:
 * - Grant requests
 * - Service IDs
 * - Scopes
 * - Expiration dates
 */

/**
 * Validates a grant creation/update request.
 * Returns array of error messages (empty if valid).
 *
 * @param request - Grant request to validate
 * @param request.grantedPermissionSets - Selected permission sets with included service IDs
 * @param request.validUntil - Optional grant expiration date
 * @param request.requireAtLeastOneService - When true (default), at least one permission set
 *   is required. Pass false for agents that have only optional service requirements, where
 *   approval without selecting any services is permitted.
 * @returns Array of validation error messages
 */
export function validateGrantRequest(request: {
  grantedPermissionSets?: Record<string, string[]>;
  validUntil?: string | null;
  requireAtLeastOneService?: boolean;
}): string[] {
  const errors: string[] = [];

  // Must have grantedPermissionSets field
  if (!request.grantedPermissionSets) {
    errors.push('grantedPermissionSets field is required');
    return errors;
  }

  // Require at least one permission set only when mandatory service requirements exist
  const psIds = Object.keys(request.grantedPermissionSets);
  if ((request.requireAtLeastOneService ?? true) && psIds.length === 0) {
    errors.push('Please select at least one permission set');
    return errors;
  }

  // Validate each permission set entry
  psIds.forEach((psId, index) => {
    if (!psId || typeof psId !== 'string') {
      errors.push(`Permission set ${index + 1}: invalid ID`);
    }
    const serviceIds = request.grantedPermissionSets![psId];
    if (!Array.isArray(serviceIds) || serviceIds.length === 0) {
      errors.push(`Permission set ${psId}: must include at least one service`);
    }
  });

  // Validate expiration date if provided
  if (request.validUntil) {
    const validationError = validateExpirationDate(
      new Date(request.validUntil),
    );
    if (validationError) {
      errors.push(validationError);
    }
  }

  return errors;
}

/**
 * Validates that a service ID is in valid format.
 *
 * @param serviceId - Service ID to validate
 * @returns True if valid, false otherwise
 */
export function validateServiceId(serviceId: string): boolean {
  // Service ID must be non-empty string
  if (!serviceId || typeof serviceId !== 'string') {
    return false;
  }

  // Service ID should not contain whitespace or special characters
  // Allow alphanumeric, hyphens, underscores, dots
  const serviceIdPattern = /^[a-zA-Z0-9._-]+$/;
  return serviceIdPattern.test(serviceId);
}

/**
 * Validates an array of OAuth scopes.
 *
 * @param scopes - Array of scope strings to validate
 * @returns True if all scopes are valid, false otherwise
 */
export function validateScopes(scopes: string[]): boolean {
  if (!Array.isArray(scopes)) {
    return false;
  }

  // Each scope must be non-empty string
  return scopes.every((scope) => {
    if (!scope || typeof scope !== 'string') {
      return false;
    }

    // Scope should not be just whitespace
    if (scope.trim().length === 0) {
      return false;
    }

    return true;
  });
}

/**
 * Validates an expiration date.
 *
 * @param date - Date to validate
 * @returns Error message if invalid, null if valid
 */
export function validateExpirationDate(date: Date): string | null {
  // Must be a valid date
  if (!(date instanceof Date) || isNaN(date.getTime())) {
    return 'Invalid date format';
  }

  // Must be in the future
  const now = new Date();
  if (date <= now) {
    return 'End date must be in the future';
  }

  // Should not be more than 10 years in the future (sanity check)
  const tenYearsFromNow = new Date();
  tenYearsFromNow.setFullYear(tenYearsFromNow.getFullYear() + 10);
  if (date > tenYearsFromNow) {
    return 'End date cannot be more than 10 years in the future';
  }

  return null;
}

/**
 * Formats validation errors for display.
 *
 * @param errors - Array of error messages
 * @returns Formatted error string
 */
export function formatValidationErrors(errors: string[]): string {
  if (errors.length === 0) {
    return '';
  }

  if (errors.length === 1) {
    return errors[0];
  }

  // Multiple errors: format as numbered list
  return errors.map((error, index) => `${index + 1}. ${error}`).join('\n');
}

/** Browser URL parsing also normalizes whitespace and backslashes. */
export function isSafeRedirectUrl(redirectUrl: string): boolean {
  if (!redirectUrl || redirectUrl.startsWith('//')) return false;
  try {
    const url = new URL(redirectUrl, window.location.origin);
    return url.protocol === window.location.protocol && url.origin === window.location.origin;
  } catch {
    return false;
  }
}
