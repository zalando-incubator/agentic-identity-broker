//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	emulator, err := bootstrap.StartAWSEmulatorForSuite(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "AWS emulator unavailable: %v\n", err)
		if os.Getenv("CI") != "" {
			os.Exit(1)
		}
	} else {
		sharedEmulator = emulator
	}

	code := m.Run()

	if sharedEmulator != nil {
		if err := sharedEmulator.Terminate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "failed to terminate AWS emulator container: %v\n", err)
		}
		if err := sharedEmulator.CleanupEnvironment(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to restore AWS emulator environment: %v\n", err)
		}
	}
	if err := bootstrap.TerminateSharedPostgres(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to terminate shared PostgreSQL container: %v\n", err)
	}

	os.Exit(code)
}
