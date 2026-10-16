package trustanchor

import "errors"

// Category classifies a key-source failure. Messages built from a Category never contain
// key-set content or library error text.
type Category string

// Key-source failure categories.
const (
	CategoryConflictingKeySources   Category = "conflicting_key_sources"
	CategoryRefreshBoundsWithInline Category = "refresh_bounds_with_inline"
	CategoryVerificationDisabled    Category = "verification_disabled_with_key_source"
	CategoryUnsupportedTrustAnchor  Category = "unsupported_trust_anchor"
	CategoryRelativePath            Category = "relative_path"
	CategoryMissingTrustDirectory   Category = "missing_trust_directory"
	CategoryInvalidTrustDirectory   Category = "invalid_trust_directory"
	CategoryOutsideTrustDirectory   Category = "outside_trust_directory"
	CategoryNotFound                Category = "not_found"
	CategoryUnreadable              Category = "unreadable"
	CategoryNotRegularFile          Category = "not_regular_file"
	CategoryTooLarge                Category = "too_large"
	CategoryNotAJWKSet              Category = "not_a_jwk_set"
	CategoryEmptyKeySet             Category = "empty_key_set"
	CategorySymmetricKey            Category = "symmetric_key"
	CategoryPrivateKey              Category = "private_key"
	CategoryUnapprovedAlgorithm     Category = "unapproved_algorithm"
	CategoryDuplicateKID            Category = "duplicate_kid"
	CategoryStale                   Category = "stale"
)

// Error is a categorized, content-free key-source failure.
type Error struct {
	Category Category
	// Path is the configuration path the error is reported against.
	Path string
	// Detail carries the non-secret values the message names, such as a kid, file path, or directory.
	Detail map[string]string
}

var errNotImplemented = errors.New("not implemented")

// Error returns "<path>: <message>".
func (e *Error) Error() string {
	return errNotImplemented.Error()
}
