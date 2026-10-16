package trustanchor

import "github.com/lestrrat-go/jwx/v4/jwk"

// MaxKeySetBytes is the size bound for any key-set document, remote or local.
const MaxKeySetBytes = 1 << 20

// LocalKeySet is a validated JWK Set from an inline or file key source.
type LocalKeySet struct {
	Set jwk.Set
	// KeyIDs lists the non-empty kid values in document order. It is used only for logs.
	KeyIDs []string
}

// ParseLocalKeySet validates doc against the Local Key Set invariants.
func ParseLocalKeySet(doc []byte) (LocalKeySet, error) {
	_ = doc
	return LocalKeySet{}, errNotImplemented
}

// DiffKeyIDs returns the key identifiers added to and removed from previous by next.
func DiffKeyIDs(previous, next LocalKeySet) (added, removed []string) {
	_, _ = previous, next
	return nil, nil
}
