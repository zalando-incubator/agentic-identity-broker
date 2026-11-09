package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
	collogpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type partialSuccessCollector struct {
	collogpb.UnimplementedLogsServiceServer
	requests  chan *collogpb.ExportLogsServiceRequest
	responses chan *collogpb.ExportLogsServiceResponse
}

func (c *partialSuccessCollector) Export(_ context.Context, request *collogpb.ExportLogsServiceRequest) (*collogpb.ExportLogsServiceResponse, error) {
	c.requests <- request
	return <-c.responses, nil
}

func TestBusinessEventPartialSuccessRetainsDeliveryReference(t *testing.T) {
	for _, protocol := range []string{ports.OTLPProtocolHTTP, ports.OTLPProtocolGRPC} {
		t.Run(protocol, func(t *testing.T) {
			t.Parallel()
			collector := &partialSuccessCollector{
				requests:  make(chan *collogpb.ExportLogsServiceRequest, 2),
				responses: make(chan *collogpb.ExportLogsServiceResponse, 2),
			}
			collector.responses <- &collogpb.ExportLogsServiceResponse{PartialSuccess: &collogpb.ExportLogsPartialSuccess{RejectedLogRecords: 1}}
			cfg := ports.DefaultTelemetryConfig()
			cfg.Exporter.Protocol = protocol
			cfg.Exporter.Insecure = true
			cfg.Exporter.Timeout = time.Second
			if protocol == ports.OTLPProtocolHTTP {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					request := new(collogpb.ExportLogsServiceRequest)
					if proto.Unmarshal(body, request) != nil {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					response, _ := collector.Export(r.Context(), request)
					encoded, _ := proto.Marshal(response)
					w.Header().Set("Content-Type", "application/x-protobuf")
					_, _ = w.Write(encoded)
				}))
				t.Cleanup(server.Close)
				cfg.Exporter.Endpoint = server.URL
			} else {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				server := grpc.NewServer()
				collogpb.RegisterLogsServiceServer(server, collector)
				go func() { _ = server.Serve(listener) }()
				t.Cleanup(server.Stop)
				cfg.Exporter.Endpoint = listener.Addr().String()
			}
			provider, err := telemetry.NewBusinessEventProvider(t.Context(), cfg)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
			store := businessEventSchemaStorage(t)
			key := deliveryTestSeed(t, store)
			delivered, err := store.BusinessEventDelivery().DispatchOne(t.Context(), key, provider.Emit)
			require.Error(t, err)
			require.False(t, delivered)
			require.Len(t, collector.requests, 1)
			request := <-collector.requests
			records := request.GetResourceLogs()[0].GetScopeLogs()[0].GetLogRecords()
			require.Len(t, records, 1)
			events := deliveryTestQuery(t, store, "pre-existing-delivery")
			require.Len(t, events, 1)
			require.Equal(t, key.ID, events[0].ID)
			// A rejected export reschedules, rather than removing, this exact key.
			require.Eventually(t, func() bool {
				return len(deliveryTestPending(t, store.BusinessEventDelivery())) == 1
			}, 35*time.Second, 100*time.Millisecond)
			require.Equal(t, key, deliveryTestPending(t, store.BusinessEventDelivery())[0])
			collector.responses <- &collogpb.ExportLogsServiceResponse{}
			delivered, err = store.BusinessEventDelivery().DispatchOne(t.Context(), key, provider.Emit)
			require.NoError(t, err)
			require.True(t, delivered)
			require.Len(t, collector.requests, 1)
			retry := <-collector.requests
			require.Equal(t, records[0].GetAttributes(), retry.GetResourceLogs()[0].GetScopeLogs()[0].GetLogRecords()[0].GetAttributes())
			require.Empty(t, deliveryTestPending(t, store.BusinessEventDelivery()))
		})
	}
}
