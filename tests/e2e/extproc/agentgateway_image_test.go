package extproc_test

import "testing"

func TestAgentgatewayTestImage(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  string
	}{
		{
			name: "uses the pinned image by default",
			want: "cr.agentgateway.dev/agentgateway:v1.7.0",
		},
		{
			name:  "uses the configured image",
			image: "cr.agentgateway.dev/agentgateway:v1.1.0",
			want:  "cr.agentgateway.dev/agentgateway:v1.1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AGENTGATEWAY_IMAGE", tt.image)

			if got := agentgatewayTestImage(); got != tt.want {
				t.Errorf("agentgatewayTestImage() = %q, want %q", got, tt.want)
			}
		})
	}
}
