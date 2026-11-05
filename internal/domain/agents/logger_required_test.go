package agents

import (
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/cascadefixture"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/assert"
)

func TestNewServiceRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "agents.NewService: logger must not be nil", func() {
		NewService(nil, nil, nil, false, ledgerfixture.NewRecorder(), cascadefixture.NewAgentDependents(), cascadefixture.NewCredentialRepository())
	})
}
