package cimdclient

import "errors"

var (
	// ErrUnsupportedCIMDAlgorithm reports an algorithm outside the ES256-only CIMD key domain.
	ErrUnsupportedCIMDAlgorithm = errors.New("CIMD client-authentication keys require ES256")
)
