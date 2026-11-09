package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDiscoveryStatus_ActiveSourceAndAttemptTransitions(t *testing.T) {
	t.Parallel()

	resourceURL := "https://mcp.example.test/mcp"
	success := time.Date(2026, time.October, 7, 10, 0, 0, 0, time.UTC)
	failedAttempt := success.Add(time.Hour)
	reason := "authorization_server_metadata_invalid"

	for _, tc := range []struct {
		name        string
		resourceURL *string
		status      DiscoveryStatus
		want        string
	}{
		{
			name: "manual service has no discovery outcome",
			want: "not_applicable",
		},
		{
			name:        "successful setup records one attempt and one success",
			resourceURL: &resourceURL,
			status:      DiscoveryStatus{LastAttemptAt: &success, LastSuccessAt: &success},
			want:        "ready",
		},
		{
			name:        "failed refresh keeps the previous success",
			resourceURL: &resourceURL,
			status:      DiscoveryStatus{LastAttemptAt: &failedAttempt, LastSuccessAt: &success, FailureReason: &reason},
			want:        "failed",
		},
		{
			name:        "next success clears the failure",
			resourceURL: &resourceURL,
			status:      DiscoveryStatus{LastAttemptAt: &failedAttempt, LastSuccessAt: &failedAttempt},
			want:        "ready",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.status.Status(tc.resourceURL))
		})
	}
}
