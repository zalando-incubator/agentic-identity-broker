//go:build integration
// +build integration

package migrations

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
)

func TestMain(m *testing.M) {
	code := m.Run()

	if err := bootstrap.TerminateSharedPostgres(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "failed to terminate shared PostgreSQL container: %v\n", err)
	}

	os.Exit(code)
}
