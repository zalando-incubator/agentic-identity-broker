package jwtauth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewCELEvaluatorRequiresLogger(t *testing.T) {
	evaluator, err := NewCELEvaluator(CELEvaluatorConfig{}, nil)

	require.EqualError(t, err, "logger must not be nil")
	require.Nil(t, evaluator)
}
