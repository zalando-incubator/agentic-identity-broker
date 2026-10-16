// Package trustanchor holds the key-source rules shared by every trust anchor: which of
// jwks_uri, jwks, and jwks_file a trust anchor uses, and the invariants of a local key set.
package trustanchor

// IsApprovedAlgorithm reports whether name is in the broker-approved asymmetric signature set.
func IsApprovedAlgorithm(name string) bool {
	_ = name
	return false
}
