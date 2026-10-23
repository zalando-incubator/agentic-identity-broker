// Package trustanchor holds the key-source rules shared by every trust anchor: which of
// jwks_uri, jwks, and jwks_file a trust anchor uses, and the invariants of a local key set.
package trustanchor

// approvedAlgorithms is the broker-approved asymmetric signature set. none and every symmetric
// (HS*) algorithm are always rejected.
var approvedAlgorithms = map[string]struct{}{
	"RS256": {}, "RS384": {}, "RS512": {},
	"PS256": {}, "PS384": {}, "PS512": {},
	"ES256": {}, "ES384": {}, "ES512": {},
	"EdDSA": {},
}

// IsApprovedAlgorithm reports whether name is in the broker-approved asymmetric signature set.
// Names are case-sensitive, as in JWS.
func IsApprovedAlgorithm(name string) bool {
	_, ok := approvedAlgorithms[name]
	return ok
}
