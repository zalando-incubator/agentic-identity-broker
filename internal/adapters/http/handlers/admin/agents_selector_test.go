package admin

import (
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/assert"
)

func TestExternalIDQueryParsing(t *testing.T) {
	for _, tc := range []struct {
		raw          string
		value        id.ExternalID
		present, bad bool
	}{
		{"", "", false, false}, {"externalId=x", "", false, false},
		{"external_id", "", true, false}, {"external_id=", "", true, false},
		{"external_id=x&%65xternal_id=y", "", true, true},
		{"%65xternal_id=x%3By", "x;y", true, false},
		{"external_id=x;y", "", true, true},
		{"external_id=%ZZ", "", true, true},
		{"external_id=literal+space", "literal space", true, false},
		{"unknown=%ZZ&external_id=registry&other=a;b", "registry", true, false},
		{"%ZZ=bad&external_id=registry", "registry", true, false},
		{"external_id=registry&unknown=%", "registry", true, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, present, message := externalIDFromQuery(tc.raw)
			assert.Equal(t, tc.value, got)
			assert.Equal(t, tc.present, present)
			assert.Equal(t, tc.bad, message != "")
		})
	}
}
