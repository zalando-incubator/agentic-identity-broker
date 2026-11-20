package matchers

import (
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

// HaveBusinessEvent matches a stored event for the policy-rendered principal.
func HaveBusinessEvent(event, principal string) types.GomegaMatcher {
	return gomega.ContainElement(gomega.SatisfyAll(
		gomega.HaveField("Event", gomega.Equal(event)),
		gomega.HaveField("Principal", gomega.HaveValue(gomega.Equal(principal))),
	))
}
