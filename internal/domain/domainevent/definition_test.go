package domainevent

import (
	"fmt"
	"log/slog"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDefinitionSequence atomic.Uint64

func TestDefinitionContract(t *testing.T) {
	name := fmt.Sprintf("test.definition.recorded_%d", testDefinitionSequence.Add(1))
	definition := MustDefine(name)
	assert.Equal(t, name, definition.Name())
	assert.Equal(t, name, definition.String())
	assert.Equal(t, slog.StringValue(name), definition.LogValue())
	assert.False(t, definition.OwnerVisible())
	visible := MustDefine(fmt.Sprintf("test.definition.visible_%d", testDefinitionSequence.Add(1)), OwnerVisible())
	assert.True(t, visible.OwnerVisible())
	assert.Panics(t, func() { MustDefine(name) })
	var zero Definition
	assert.Empty(t, zero.Name())
	assert.False(t, zero.OwnerVisible())
	definitions := Definitions()
	names := make([]string, len(definitions))
	found := map[string]bool{}
	for i, d := range definitions {
		names[i] = d.Name()
		found[d.Name()] = d.OwnerVisible()
	}
	assert.True(t, sort.StringsAreSorted(names))
	require.Contains(t, found, name)
	assert.False(t, found[name])
	assert.True(t, found[visible.Name()])
	definitions[0] = zero
	assert.NotEqual(t, definitions, Definitions(), "registry results must not alias its state")
}

func TestDefinitionKeepsValidatedName(t *testing.T) {
	suffix := testDefinitionSequence.Add(1)
	previous := MustDefine(fmt.Sprintf("test.definition.original_%d", suffix))
	name := fmt.Sprintf("test.definition.next_%d", suffix)
	got := MustDefine(name, func(d *Definition) { *d = previous }, OwnerVisible())
	assert.Equal(t, name, got.Name())
	assert.True(t, got.OwnerVisible())
	assert.Contains(t, Definitions(), got)
}

func TestDefinitionRejectsMalformedIdentifiers(t *testing.T) {
	for _, name := range []string{"", "consent", "consent.grant", "consent.grant.revoked.extra", "Consent.grant.revoked", "consent.grant.re-voked", "1consent.grant.revoked", "consent..revoked", "consent.grant.revoked\n"} {
		t.Run(name, func(t *testing.T) {
			assert.Panics(t, func() { MustDefine(name) })
		})
	}
}
