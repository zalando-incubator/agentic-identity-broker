package trustanchor

import (
	"errors"
	"fmt"
	"strconv"
)

// Category classifies a key-source failure. Messages built from a Category never contain
// key-set content or library error text.
type Category string

// Key-source failure categories.
const (
	CategoryConflictingKeySources   Category = "conflicting_key_sources"
	CategoryRefreshBoundsWithInline Category = "refresh_bounds_with_inline"
	CategoryVerificationDisabled    Category = "verification_disabled_with_key_source"
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

// Detail keys name the non-secret values that a message can include.
const (
	DetailFound   = "found"
	DetailSetting = "setting"
	DetailPath    = "path"
	DetailDir     = "dir"
	DetailKID     = "kid"
	DetailIndex   = "index"
)

// Error is a categorized, content-free key-source failure.
type Error struct {
	Category Category
	// Path is the configuration path the error is reported against.
	Path string
	// Detail carries the non-secret values the message names, such as a kid, file path, or directory.
	Detail map[string]string
}

// Error returns "<path>: <message>", or the bare message when no path is set.
func (e *Error) Error() string {
	if e.Path == "" {
		return e.message()
	}
	return e.Path + ": " + e.message()
}

// Message returns the category description without the configuration path.
func (e *Error) Message() string {
	return e.message()
}

func (e *Error) message() string {
	d := e.Detail
	switch e.Category {
	case CategoryConflictingKeySources:
		return "only one of jwks_uri, jwks, jwks_file may be set (found: " + d[DetailFound] + ")"
	case CategoryRefreshBoundsWithInline:
		return "jwks_min_refresh and jwks_max_refresh are not allowed with jwks; an inline key set never refreshes"
	case CategoryVerificationDisabled:
		return "verification 'none' and " + d[DetailSetting] + " are mutually exclusive"
	case CategoryRelativePath:
		return fmt.Sprintf("jwks_file %q must be an absolute path", d[DetailPath])
	case CategoryMissingTrustDirectory:
		return "jwks_file requires security.trust_material_dir to be set"
	case CategoryInvalidTrustDirectory:
		return fmt.Sprintf("security.trust_material_dir %q must be an absolute path to an existing directory other than /", d[DetailDir])
	case CategoryOutsideTrustDirectory:
		return fmt.Sprintf("jwks_file %q is outside security.trust_material_dir %q", d[DetailPath], d[DetailDir])
	case CategoryNotFound:
		return fmt.Sprintf("jwks_file %q does not exist", d[DetailPath])
	case CategoryUnreadable:
		return fmt.Sprintf("jwks_file %q cannot be read", d[DetailPath])
	case CategoryNotRegularFile:
		return fmt.Sprintf("jwks_file %q is not a regular file", d[DetailPath])
	case CategoryTooLarge:
		return "key set exceeds " + strconv.Itoa(MaxKeySetBytes) + " bytes"
	case CategoryNotAJWKSet:
		return "key set is not a valid JWK Set document"
	case CategoryEmptyKeySet:
		return "key set contains no keys"
	case CategorySymmetricKey:
		return "key set contains a symmetric key; only public asymmetric keys are allowed"
	case CategoryPrivateKey:
		return "key set contains private key material; only public keys are allowed"
	case CategoryUnapprovedAlgorithm:
		if kid := d[DetailKID]; kid != "" {
			return fmt.Sprintf("key %q declares an algorithm outside the approved asymmetric set", kid)
		}
		return "key at index " + d[DetailIndex] + " declares an algorithm outside the approved asymmetric set"
	case CategoryDuplicateKID:
		return fmt.Sprintf("key identifier %q appears more than once", d[DetailKID])
	case CategoryStale:
		return "key set has not been read successfully within jwks_max_refresh"
	default:
		return string(e.Category)
	}
}

func newError(category Category, detail ...string) *Error {
	err := &Error{Category: category}
	if len(detail) > 0 {
		err.Detail = make(map[string]string, len(detail)/2)
		for i := 0; i+1 < len(detail); i += 2 {
			err.Detail[detail[i]] = detail[i+1]
		}
	}
	return err
}

// At reports err against a configuration path. A trust-anchor error that already names a path
// keeps it; any other error is prefixed with the path.
func At(path string, err error) error {
	var anchorErr *Error
	if errors.As(err, &anchorErr) {
		if anchorErr.Path != "" {
			return err
		}
		located := *anchorErr
		located.Path = path
		return &located
	}
	return fmt.Errorf("%s: %w", path, err)
}

// CategoryOf returns the category of a trust-anchor error, or "" for any other error.
func CategoryOf(err error) Category {
	var anchorErr *Error
	if errors.As(err, &anchorErr) {
		return anchorErr.Category
	}
	return ""
}
