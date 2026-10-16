package trustanchor

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAnchorPath = "token_exchange.client_assertion"
	testTrustDir   = "/etc/agentic-identity-broker/trust"
	testInlineSet  = `{"keys":[]}`
)

var clientAssertionAnchor = AnchorRef{Kind: AnchorClientAssertion, ConfigPath: testAnchorPath}

func requireCategory(t *testing.T, err error, want Category) *Error {
	t.Helper()
	require.Error(t, err)
	var anchorErr *Error
	require.True(t, errors.As(err, &anchorErr), "expected *trustanchor.Error, got %T", err)
	assert.Equal(t, want, anchorErr.Category)
	return anchorErr
}

func TestResolveKeySetSourceStructuralRules(t *testing.T) {
	tests := []struct {
		name    string
		anchor  AnchorRef
		input   SourceInput
		want    Category
		message string
	}{
		{
			name:    "rule 1: jwks_uri and jwks",
			input:   SourceInput{JWKSURI: "https://idp.example.com/keys", JWKS: testInlineSet},
			want:    CategoryConflictingKeySources,
			message: testAnchorPath + ": only one of jwks_uri, jwks, jwks_file may be set (found: jwks_uri, jwks)",
		},
		{
			name:    "rule 1: all three sources",
			input:   SourceInput{JWKSURI: "https://idp.example.com/keys", JWKS: testInlineSet, JWKSFile: testTrustDir + "/a.json", TrustDirectory: testTrustDir},
			want:    CategoryConflictingKeySources,
			message: testAnchorPath + ": only one of jwks_uri, jwks, jwks_file may be set (found: jwks_uri, jwks, jwks_file)",
		},
		{
			name:    "rule 1: jwks and jwks_file",
			input:   SourceInput{JWKS: testInlineSet, JWKSFile: testTrustDir + "/a.json", TrustDirectory: testTrustDir},
			want:    CategoryConflictingKeySources,
			message: testAnchorPath + ": only one of jwks_uri, jwks, jwks_file may be set (found: jwks, jwks_file)",
		},
		{
			name:    "rule 2: inline with explicit minimum refresh",
			input:   SourceInput{JWKS: testInlineSet, MinRefresh: time.Minute},
			want:    CategoryRefreshBoundsWithInline,
			message: testAnchorPath + ": jwks_min_refresh and jwks_max_refresh are not allowed with jwks; an inline key set never refreshes",
		},
		{
			name:  "rule 2: inline with explicit maximum refresh",
			input: SourceInput{JWKS: testInlineSet, MaxRefresh: time.Hour},
			want:  CategoryRefreshBoundsWithInline,
		},
		{
			name:    "rule 3: verification none with jwks",
			anchor:  AnchorRef{Kind: AnchorJWTPreauth, ConfigPath: "server.enduser.authentication.jwt"},
			input:   SourceInput{JWKS: testInlineSet, VerificationDisabled: true},
			want:    CategoryVerificationDisabled,
			message: "server.enduser.authentication.jwt: verification 'none' and jwks are mutually exclusive",
		},
		{
			name:    "rule 3: verification none with jwks_file",
			anchor:  AnchorRef{Kind: AnchorJWTPreauth, ConfigPath: "server.admin.authentication.jwt"},
			input:   SourceInput{JWKSFile: testTrustDir + "/mesh.json", TrustDirectory: testTrustDir, VerificationDisabled: true},
			want:    CategoryVerificationDisabled,
			message: "server.admin.authentication.jwt: verification 'none' and jwks_file are mutually exclusive",
		},
		{
			name:    "rule 3: verification none with jwks_uri",
			anchor:  AnchorRef{Kind: AnchorJWTPreauth, ConfigPath: "server.enduser.authentication.jwt"},
			input:   SourceInput{JWKSURI: "https://idp.example.com/keys", VerificationDisabled: true},
			want:    CategoryVerificationDisabled,
			message: "server.enduser.authentication.jwt: verification 'none' and jwks_uri are mutually exclusive",
		},
		{
			name:    "rule 4: relative jwks_file",
			input:   SourceInput{JWKSFile: "trust/a.json", TrustDirectory: testTrustDir},
			want:    CategoryRelativePath,
			message: testAnchorPath + `: jwks_file "trust/a.json" must be an absolute path`,
		},
		{
			name:    "rule 5: jwks_file without trust directory",
			input:   SourceInput{JWKSFile: testTrustDir + "/a.json"},
			want:    CategoryMissingTrustDirectory,
			message: testAnchorPath + ": jwks_file requires security.trust_material_dir to be set",
		},
		{
			name:    "rule 6: jwks_file outside the trust directory",
			input:   SourceInput{JWKSFile: "/etc/agentic-identity-broker/config.yaml", TrustDirectory: testTrustDir},
			want:    CategoryOutsideTrustDirectory,
			message: testAnchorPath + `: jwks_file "/etc/agentic-identity-broker/config.yaml" is outside security.trust_material_dir "/etc/agentic-identity-broker/trust"`,
		},
		{
			name:    "rule 6: dot-dot escape is cleaned before the check",
			input:   SourceInput{JWKSFile: testTrustDir + "/../config.yaml", TrustDirectory: testTrustDir},
			want:    CategoryOutsideTrustDirectory,
			message: testAnchorPath + `: jwks_file "/etc/agentic-identity-broker/trust/../config.yaml" is outside security.trust_material_dir "/etc/agentic-identity-broker/trust"`,
		},
		{
			name:  "rule 6: sibling directory sharing a prefix",
			input: SourceInput{JWKSFile: testTrustDir + "-other/a.json", TrustDirectory: testTrustDir},
			want:  CategoryOutsideTrustDirectory,
		},
		{
			name:  "rule 6: the trust directory itself",
			input: SourceInput{JWKSFile: testTrustDir, TrustDirectory: testTrustDir},
			want:  CategoryOutsideTrustDirectory,
		},
		{
			name:    "trust directory must be absolute",
			input:   SourceInput{JWKSFile: "/trust/a.json", TrustDirectory: "trust"},
			want:    CategoryInvalidTrustDirectory,
			message: `security.trust_material_dir "trust" must be an absolute path to an existing directory other than /`,
		},
		{
			name:  "trust directory must not be the filesystem root",
			input: SourceInput{JWKSFile: "/a.json", TrustDirectory: "/"},
			want:  CategoryInvalidTrustDirectory,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			anchor := tc.anchor
			if anchor.ConfigPath == "" {
				anchor = clientAssertionAnchor
			}
			_, err := ResolveKeySetSource(anchor, tc.input)
			anchorErr := requireCategory(t, err, tc.want)
			if tc.message != "" {
				assert.Equal(t, tc.message, anchorErr.Error())
			}
		})
	}
}

func TestResolveKeySetSourceKinds(t *testing.T) {
	t.Run("rule 7: jwks selects inline", func(t *testing.T) {
		source, err := ResolveKeySetSource(clientAssertionAnchor, SourceInput{JWKS: testInlineSet})
		require.NoError(t, err)
		assert.Equal(t, SourceInline, source.Kind)
		assert.Equal(t, []byte(testInlineSet), source.Document)
	})

	t.Run("rule 8: jwks_file selects file with a cleaned path", func(t *testing.T) {
		source, err := ResolveKeySetSource(clientAssertionAnchor, SourceInput{
			JWKSFile: testTrustDir + "/./sub/../gateway.json", TrustDirectory: testTrustDir + "/",
		})
		require.NoError(t, err)
		assert.Equal(t, SourceFile, source.Kind)
		assert.Equal(t, testTrustDir+"/gateway.json", source.Path)
	})

	t.Run("rule 9: jwks_uri selects remote", func(t *testing.T) {
		source, err := ResolveKeySetSource(clientAssertionAnchor, SourceInput{JWKSURI: "https://idp.example.com/keys"})
		require.NoError(t, err)
		assert.Equal(t, SourceRemote, source.Kind)
		assert.Equal(t, "https://idp.example.com/keys", source.URI)
	})

	t.Run("rule 10: no explicit source keeps the existing behaviour", func(t *testing.T) {
		source, err := ResolveKeySetSource(clientAssertionAnchor, SourceInput{})
		require.NoError(t, err)
		assert.Equal(t, SourceImplicit, source.Kind)
	})

	t.Run("a trust directory without jwks_file is never validated", func(t *testing.T) {
		source, err := ResolveKeySetSource(clientAssertionAnchor, SourceInput{JWKS: testInlineSet, TrustDirectory: "relative"})
		require.NoError(t, err)
		assert.Equal(t, SourceInline, source.Kind)
	})
}

func TestResolveKeySetSourceRefreshBounds(t *testing.T) {
	tests := []struct {
		name   string
		anchor AnchorKind
		min    time.Duration
		max    time.Duration
		want   RefreshBounds
	}{
		{name: "client assertion defaults", anchor: AnchorClientAssertion,
			want: RefreshBounds{Min: 15 * time.Minute, Max: time.Hour}},
		{name: "impersonation defaults", anchor: AnchorImpersonationIssuer,
			want: RefreshBounds{Min: 15 * time.Minute, Max: time.Hour}},
		{name: "explicit bounds", anchor: AnchorClientAssertion, min: 200 * time.Millisecond, max: 2 * time.Second,
			want: RefreshBounds{Min: 200 * time.Millisecond, Max: 2 * time.Second, Explicit: true}},
		{name: "minimum above one hour lifts the default maximum", anchor: AnchorClientAssertion, min: 2 * time.Hour,
			want: RefreshBounds{Min: 2 * time.Hour, Max: 2 * time.Hour, Explicit: true}},
		{name: "maximum below minimum is raised as before", anchor: AnchorClientAssertion, min: 30 * time.Minute, max: time.Minute,
			want: RefreshBounds{Min: 30 * time.Minute, Max: time.Hour, Explicit: true}},
		{name: "JWT pre-authentication is fixed", anchor: AnchorJWTPreauth,
			want: RefreshBounds{Min: 5 * time.Minute, Max: 15 * time.Minute}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source, err := ResolveKeySetSource(AnchorRef{Kind: tc.anchor, ConfigPath: "anchor"}, SourceInput{
				JWKSFile: testTrustDir + "/a.json", TrustDirectory: testTrustDir, MinRefresh: tc.min, MaxRefresh: tc.max,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.want, source.Refresh)
			assert.Positive(t, source.Refresh.Min)
			assert.LessOrEqual(t, source.Refresh.Min, source.Refresh.Max)
		})
	}
}

func TestValidateTrustDirectory(t *testing.T) {
	for _, dir := range []string{"/etc/trust", "/etc/trust/", "/var/run/secrets/jwks"} {
		assert.NoError(t, ValidateTrustDirectory(dir), dir)
	}
	for _, dir := range []string{"", "trust", "./trust", "/", "//", "/etc/.."} {
		_ = requireCategory(t, ValidateTrustDirectory(dir), CategoryInvalidTrustDirectory)
	}
}

func TestErrorAt(t *testing.T) {
	err := At(testAnchorPath+".jwks", &Error{Category: CategoryEmptyKeySet})
	assert.Equal(t, testAnchorPath+".jwks: key set contains no keys", err.Error())

	located := &Error{Category: CategoryEmptyKeySet, Path: "first"}
	assert.Equal(t, "first: key set contains no keys", At("second", located).Error(), "an existing path is kept")

	other := errors.New("boom")
	assert.Equal(t, "path: boom", At("path", other).Error())
	assert.ErrorIs(t, At("path", other), other)
}
