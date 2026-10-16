package trustanchor

import (
	"path/filepath"
	"strings"
	"time"
)

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

// Refresh defaults. Client-assertion and impersonation anchors follow ADR 029; JWT
// pre-authentication keeps the fixed cadence of its remote key-set cache.
const (
	DefaultMinRefresh      = 15 * time.Minute
	DefaultMaxRefreshFloor = time.Hour
	JWTPreauthMinRefresh   = 5 * time.Minute
	JWTPreauthMaxRefresh   = 15 * time.Minute
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
	// Path is the cleaned absolute jwks_file path.
	Path    string
	Refresh RefreshBounds
}

// ResolveKeySetSource applies the structural resolution rules for one trust anchor. It performs
// no I/O: file existence and symbolic-link containment are checked when the file is opened.
func ResolveKeySetSource(anchor AnchorRef, in SourceInput) (KeySetSource, error) {
	var found []string
	for _, setting := range []struct{ name, value string }{
		{"jwks_uri", in.JWKSURI}, {"jwks", in.JWKS}, {"jwks_file", in.JWKSFile},
	} {
		if setting.value != "" {
			found = append(found, setting.name)
		}
	}
	if len(found) > 1 {
		return KeySetSource{}, at(anchor, newError(CategoryConflictingKeySources, DetailFound, strings.Join(found, ", ")))
	}
	if in.JWKS != "" && (in.MinRefresh != 0 || in.MaxRefresh != 0) {
		return KeySetSource{}, at(anchor, newError(CategoryRefreshBoundsWithInline))
	}
	if in.VerificationDisabled && len(found) == 1 {
		return KeySetSource{}, at(anchor, newError(CategoryVerificationDisabled, DetailSetting, found[0]))
	}

	refresh := refreshBounds(anchor.Kind, in.MinRefresh, in.MaxRefresh)
	switch {
	case in.JWKS != "":
		return KeySetSource{Kind: SourceInline, Document: []byte(in.JWKS)}, nil
	case in.JWKSFile != "":
		path, err := resolveFilePath(in.JWKSFile, in.TrustDirectory)
		if CategoryOf(err) == CategoryInvalidTrustDirectory {
			return KeySetSource{}, err
		}
		if err != nil {
			return KeySetSource{}, at(anchor, err)
		}
		return KeySetSource{Kind: SourceFile, Path: path, Refresh: refresh}, nil
	case in.JWKSURI != "":
		return KeySetSource{Kind: SourceRemote, URI: in.JWKSURI, Refresh: refresh}, nil
	default:
		return KeySetSource{Kind: SourceImplicit, Refresh: refresh}, nil
	}
}

func resolveFilePath(file, trustDir string) (string, error) {
	if !filepath.IsAbs(file) {
		return "", newError(CategoryRelativePath, DetailPath, file)
	}
	if trustDir == "" {
		return "", newError(CategoryMissingTrustDirectory)
	}
	if err := ValidateTrustDirectory(trustDir); err != nil {
		return "", err
	}
	path := filepath.Clean(file)
	if !Contains(trustDir, path) {
		return "", newError(CategoryOutsideTrustDirectory, DetailPath, file, DetailDir, trustDir)
	}
	return path, nil
}

// Contains reports whether path lies strictly beneath dir after both are cleaned lexically.
func Contains(dir, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	return err == nil && rel != "." && filepath.IsLocal(rel)
}

// ValidateTrustDirectory checks the configured security.trust_material_dir string: it must be
// absolute and must not be the filesystem root. Existence is checked when it is opened.
func ValidateTrustDirectory(dir string) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) == string(filepath.Separator) {
		return newError(CategoryInvalidTrustDirectory, DetailDir, dir)
	}
	return nil
}

func refreshBounds(kind AnchorKind, minRefresh, maxRefresh time.Duration) RefreshBounds {
	if kind == AnchorJWTPreauth {
		return RefreshBounds{Min: JWTPreauthMinRefresh, Max: JWTPreauthMaxRefresh}
	}
	bounds := RefreshBounds{Min: minRefresh, Max: maxRefresh, Explicit: minRefresh != 0 || maxRefresh != 0}
	if bounds.Min == 0 {
		bounds.Min = DefaultMinRefresh
	}
	if bounds.Max == 0 || bounds.Max < bounds.Min {
		bounds.Max = max(bounds.Min, DefaultMaxRefreshFloor)
	}
	return bounds
}

func at(anchor AnchorRef, err error) error {
	return At(anchor.ConfigPath, err)
}
