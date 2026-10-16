package trustanchor

import "time"

// AnchorKind identifies the kind of trust anchor that owns a key set source.
type AnchorKind string

// Trust anchor kinds.
const (
	AnchorClientAssertion     AnchorKind = "client_assertion"
	AnchorImpersonationIssuer AnchorKind = "impersonation_issuer"
	AnchorJWTPreauth          AnchorKind = "jwt_preauth"
)

// AnchorRef names a trust anchor by its configuration path.
type AnchorRef struct {
	Kind       AnchorKind
	ConfigPath string
}

// SourceKind is the origin of a trust anchor's verification keys.
type SourceKind string

// Key set source kinds.
const (
	// SourceImplicit means no explicit source: discovery or upstream key reuse, as before.
	SourceImplicit SourceKind = "implicit"
	SourceRemote   SourceKind = "remote"
	SourceInline   SourceKind = "inline"
	SourceFile     SourceKind = "file"
)

// SourceInput carries the configuration fields that select a key set source.
type SourceInput struct {
	JWKSURI  string
	JWKS     string
	JWKSFile string
	// MinRefresh and MaxRefresh are the configured bounds; zero means unset.
	MinRefresh time.Duration
	MaxRefresh time.Duration
	// TrustDirectory is security.trust_material_dir.
	TrustDirectory string
	// VerificationDisabled is true for JWT pre-authentication with verification "none".
	VerificationDisabled bool
}

// RefreshBounds are the effective re-read or refresh bounds of a key set source.
type RefreshBounds struct {
	Min      time.Duration
	Max      time.Duration
	Explicit bool
}

// KeySetSource is a trust anchor's resolved key source.
type KeySetSource struct {
	Kind     SourceKind
	URI      string
	Document []byte
	Path     string
	Refresh  RefreshBounds
}

// ResolveKeySetSource applies the structural resolution rules for one trust anchor.
func ResolveKeySetSource(anchor AnchorRef, in SourceInput) (KeySetSource, error) {
	_, _ = anchor, in
	return KeySetSource{}, errNotImplemented
}
