package bootstrap

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequireSharedPostgresFailsInCIWithoutRuntime(t *testing.T) {
	if os.Getenv("TEST_POSTGRES_WITHOUT_RUNTIME") == "1" {
		RequireSharedPostgres(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRequireSharedPostgresFailsInCIWithoutRuntime$")
	cmd.Env = append(os.Environ(), "TEST_POSTGRES_WITHOUT_RUNTIME=1", "CI=true", "PATH="+t.TempDir())
	output, err := cmd.CombinedOutput()
	require.Error(t, err, "CI must fail, not skip, when PostgreSQL cannot start: %s", output)
}
