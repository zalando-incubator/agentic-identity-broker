package oauth2session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewOAuth2SessionServiceRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "oauth2session.NewOAuth2SessionService: logger must not be nil", func() {
		NewOAuth2SessionService(nil, nil, nil, nil, nil, nil, nil, nil, Config{}, nil, nil, nil)
	})
}

func TestNewSessionSweepServiceRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "oauth2session.NewSessionSweepService: logger must not be nil", func() {
		NewSessionSweepService(nil, nil, nil)
	})
}
