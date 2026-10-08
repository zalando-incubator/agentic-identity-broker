package telemetry

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetryResourceExcludesProcessCredentials(t *testing.T) {
	const secret = "SENTINEL_PROCESS_ARGUMENT_SECRET"
	previousArgs := os.Args
	os.Args = []string{"broker", "--client-secret=" + secret, "--token-endpoint=https://provider.example/" + secret}
	t.Cleanup(func() { os.Args = previousArgs })
	resource, err := buildResource(context.Background(), ports.TelemetryConfig{
		ServiceName: "broker",
		ResourceAttributes: map[string]string{
			"process.command_args":        secret,
			"process.command":             secret,
			"process.command_line":        secret,
			"deployment.environment.name": "test",
		},
	})
	require.NoError(t, err)
	assert.NotContains(t, fmt.Sprint(resource.Attributes()), secret)
	attrs := make(map[string]any)
	for _, attr := range resource.Attributes() {
		attrs[string(attr.Key)] = attr.Value.AsInterface()
	}
	assert.NotContains(t, attrs, "process.command_args")
	assert.NotContains(t, attrs, "process.command")
	assert.NotContains(t, attrs, "process.command_line")
	assert.Equal(t, "test", attrs["deployment.environment.name"])
	assert.Equal(t, "broker", attrs["service.name"])
}
