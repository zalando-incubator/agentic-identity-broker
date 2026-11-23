package spans

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestFixturesAreNotChecked(t *testing.T) {
	otel.Tracer("test").Start(context.Background(), "test span")
}
