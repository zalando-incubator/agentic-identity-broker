package telemetry

import (
	"context"
	"errors"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

// BusinessEventProvider owns a non-global SDK log provider, separate from slog.
type BusinessEventProvider struct {
	logger otellog.Logger
	sdk    *sdklog.LoggerProvider
}

const businessEventScope = "agentic-identity-broker.ledger"

var errBusinessEventExport = errors.New("business event telemetry export was not acknowledged")

type businessEventProcessor struct{ sdklog.Processor }

type businessEventExportKey struct{}

type businessEventExportResult struct {
	attributes int
	traceID    trace.TraceID
	spanID     trace.SpanID
	called     bool
	err        error
}

func (p *businessEventProcessor) OnEmit(ctx context.Context, record *sdklog.Record) error {
	result, ok := ctx.Value(businessEventExportKey{}).(*businessEventExportResult)
	if !ok {
		return errBusinessEventExport
	}
	result.called = true
	if err := ctx.Err(); err != nil {
		result.err = err
		return nil
	}
	if record.DroppedAttributes() != 0 || record.AttributesLen() != result.attributes {
		result.err = errBusinessEventExport
		return nil
	}
	record.SetTraceID(result.traceID)
	record.SetSpanID(result.spanID)
	record.SetTraceFlags(0)
	// Emit returns this result without sending exporter details to global SDK diagnostics.
	result.err = p.Processor.OnEmit(ctx, record)
	if err := ctx.Err(); err != nil {
		result.err = err
	}
	return nil
}

func NewBusinessEventProvider(ctx context.Context, cfg ports.TelemetryConfig) (*BusinessEventProvider, error) {
	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, errBusinessEventExport
	}
	exporter, err := buildLogExporter(ctx, cfg, nil)
	if err != nil {
		return nil, errBusinessEventExport
	}
	return newBusinessEventProvider(exporter, res), nil
}

func newBusinessEventProvider(exporter sdklog.Exporter, res *resource.Resource) *BusinessEventProvider {
	processor := &businessEventProcessor{Processor: sdklog.NewSimpleProcessor(exporter)}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(processor), sdklog.WithResource(res),
		sdklog.WithAttributeCountLimit(-1), sdklog.WithAttributeValueLengthLimit(-1),
	)
	return &BusinessEventProvider{logger: provider.Logger(businessEventScope), sdk: provider}
}

func (p *BusinessEventProvider) Emit(ctx context.Context, event *model.BusinessEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	record, attributes, err := encodeBusinessEvent(event)
	if err != nil {
		return err
	}
	result := businessEventExportResult{attributes: attributes}
	if event.TraceID != "" {
		result.traceID, err = trace.TraceIDFromHex(event.TraceID)
		if err != nil {
			return errBusinessEventEncoding
		}
	}
	if event.SpanID != "" {
		if !result.traceID.IsValid() {
			return errBusinessEventEncoding
		}
		result.spanID, err = trace.SpanIDFromHex(event.SpanID)
		if err != nil {
			return errBusinessEventEncoding
		}
	}
	p.logger.Emit(context.WithValue(ctx, businessEventExportKey{}, &result), record)
	if !result.called {
		return errBusinessEventExport
	}
	return result.err
}

func (p *BusinessEventProvider) Shutdown(ctx context.Context) error {
	return p.sdk.Shutdown(ctx)
}
