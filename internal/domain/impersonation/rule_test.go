package impersonation

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuleSubjectCategories(t *testing.T) {
	for _, tc := range []struct {
		categories []string
		valid      bool
	}{
		{nil, true}, {[]string{"human"}, true}, {[]string{"agent"}, true},
		{[]string{"human", "agent"}, true}, {[]string{"agent", "human"}, true},
		{[]string{}, false}, {[]string{"machine"}, false}, {[]string{"Human"}, false},
		{[]string{"human", "human"}, false}, {[]string{"human", "agent", "human"}, false},
	} {
		t.Run(fmt.Sprint(tc.categories), func(t *testing.T) {
			cfg := testImpersonationConfig("categories").Rules[0]
			cfg.AllowedSubjectCategories = tc.categories
			err := validateRuleInvariants(cfg)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "allowed_subject_categories")
			}
		})
	}
}
