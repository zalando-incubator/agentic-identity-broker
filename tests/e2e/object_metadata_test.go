package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

// metadataBroker is one broker under test: Admin and End-User servers over
// one store, with every telemetry signal captured.
type metadataBroker struct {
	store     *storageadapter.Adapter
	admin     *bootstrap.TestServer
	enduser   *bootstrap.TestServer
	upstream  *helpers.MockUpstreamOAuth2Server
	telemetry helpers.TelemetryCapture
	serviceID id.ServiceID
	setID     id.PermissionSetID
	agent     *storagedomain.Agent
	close     func()
	// endUserBodies records every End-User response body the runtime journey read.
	endUserBodies []string
}

type adminReply struct {
	status int
	body   map[string]any
	raw    string
}

func newMetadataBroker(store *storageadapter.Adapter, closeStore func()) *metadataBroker {
	ctx := context.Background()
	logger, logs := bootstrap.NewBufferedJSONLogger(slog.LevelDebug)
	tracerProvider, spans := bootstrap.NewInMemoryTracerProvider()
	metrics := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metrics))
	previousMeterProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(meterProvider)
	upstream := helpers.NewMockUpstreamOAuth2Server()
	service := fixtures.GitHubService()
	service.Endpoints.TokenEndpoint = upstream.URL() + "/oauth/token"
	service.Endpoints.AuthorizeEndpoint = upstream.URL() + "/oauth/authorize"
	Expect(store.Services().Create(ctx, service)).To(Succeed())
	Expect(fixtures.SeedPlaceholderGrantData(ctx, store, service.ID)).To(Succeed())
	agent := fixtures.ValidAgent()
	Expect(store.Agents().Create(ctx, agent)).To(Succeed())
	application, err := bootstrap.NewServerFactory(approvalTestConfig(upstream.URL()), logger).BuildAppWithTracerProvider(store, tracerProvider)
	Expect(err).NotTo(HaveOccurred())
	enduser, err := bootstrap.NewEndUserTestServer(application, logger)
	Expect(err).NotTo(HaveOccurred())
	admin, err := bootstrap.NewAdminTestServer(application, logger)
	Expect(err).NotTo(HaveOccurred())
	broker := &metadataBroker{
		store: store, admin: admin, enduser: enduser, upstream: upstream,
		telemetry: helpers.TelemetryCapture{Logs: logs, Spans: spans, Metrics: metrics},
		serviceID: service.ID, setID: fixtures.PlaceholderPermissionSetID, agent: agent,
	}
	broker.close = func() {
		admin.CloseListener()
		enduser.Close()
		upstream.Close()
		otel.SetMeterProvider(previousMeterProvider)
		Expect(meterProvider.Shutdown(ctx)).To(Succeed())
		Expect(tracerProvider.Shutdown(ctx)).To(Succeed())
		closeStore()
	}
	return broker
}

func newMemoryMetadataBroker() *metadataBroker {
	factory := bootstrap.NewStorageFactory(bootstrap.TestLogger(slog.LevelWarn))
	store, err := factory.NewTestStorage()
	Expect(err).NotTo(HaveOccurred())
	return newMetadataBroker(store, func() { Expect(factory.CloseStorage(store)).To(Succeed()) })
}

func newPostgresMetadataBroker() *metadataBroker {
	ctx := context.Background()
	fixture, err := bootstrap.NewPostgresFixture(ctx)
	Expect(err).NotTo(HaveOccurred())
	store := externalIDPostgresStorage(fixture)
	return newMetadataBroker(store, func() {
		Expect(store.Close(ctx)).To(Succeed())
		Expect(fixture.Close(ctx)).To(Succeed())
	})
}

func (b *metadataBroker) adminRequest(method, path, body string) adminReply {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	response, err := b.admin.DirectRequest(method, path, fixtures.AdminPrincipal().String(), map[string]string{"Content-Type": "application/json"}, reader)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	reply := adminReply{status: response.StatusCode, raw: string(raw)}
	_ = json.Unmarshal(raw, &reply.body)
	return reply
}

func (b *metadataBroker) expectAdmin(method, path, body string, status int) adminReply {
	reply := b.adminRequest(method, path, body)
	ExpectWithOffset(1, reply.status).To(Equal(status), reply.raw)
	return reply
}

func (b *metadataBroker) expectNoSentinelLeaks() {
	ExpectWithOffset(1, b.telemetry.SentinelLeaks(fixtures.MetadataSentinels())).To(BeEmpty())
}

func (b *metadataBroker) auditRecords(event string) int {
	records, err := b.telemetry.Logs.Records()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	count := 0
	for _, record := range records {
		if record["event"] == event {
			count++
		}
	}
	return count
}

func (b *metadataBroker) storedAgent(agentID string) *storagedomain.Agent {
	agent, err := b.store.Agents().Get(context.Background(), id.MustParseAgentID(agentID))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return agent
}

func metadataAgentBody(setID id.PermissionSetID, metadata string) string {
	body := `{"display_name":"Review Bot","description":"Reviews pull requests","permission_sets":[{"permission_set_id":"` +
		setID.String() + `","requirement_type":"optional"}]}`
	if metadata == "" {
		return body
	}
	return strings.TrimSuffix(body, "}") + `,"metadata":` + metadata + `}`
}

func metadataServiceBody(displayName, metadata string) string {
	body := `{"display_name":"` + displayName + `","client_id":"atlas-client","client_secret":"atlas-secret",` +
		`"issuer_uri":"https://atlas.example.test","discovery":{"enable_discovery":false},` +
		`"endpoints":{"token_endpoint":"https://atlas.example.test/token","authorize_endpoint":"https://atlas.example.test/authorize"},` +
		`"scopes":[{"scope_value":"read","description":"Read"}]}`
	if metadata == "" {
		return body
	}
	return strings.TrimSuffix(body, "}") + `,"metadata":` + metadata + `}`
}

func metadataPermissionSetBody(name string, serviceID id.ServiceID, metadata string) string {
	body := `{"name":"` + name + `","description":"Read access","service_scopes":[{"service_id":"` + serviceID.String() +
		`","scopes":["repo"],"requirement_type":"optional"}]}`
	if metadata == "" {
		return body
	}
	return strings.TrimSuffix(body, "}") + `,"metadata":` + metadata + `}`
}

// metadataResources returns a write path for each resource, each creating a
// new resource with the given metadata.
func (b *metadataBroker) metadataCreators() map[string]func(metadata string) adminReply {
	return map[string]func(string) adminReply{
		"agent": func(metadata string) adminReply {
			return b.adminRequest(http.MethodPost, "/api/agents", metadataAgentBody(b.setID, metadata))
		},
		"service": func(metadata string) adminReply {
			return b.adminRequest(http.MethodPost, "/api/services", metadataServiceBody("Atlas "+id.NewServiceID().String(), metadata))
		},
		"permission set": func(metadata string) adminReply {
			return b.adminRequest(http.MethodPost, "/api/permission-sets", metadataPermissionSetBody("Set "+id.NewPermissionSetID().String(), b.serviceID, metadata))
		},
	}
}

func (b *metadataBroker) resourceCounts() [3]int {
	ctx := context.Background()
	agents, err := b.store.Agents().List(ctx)
	Expect(err).NotTo(HaveOccurred())
	services, err := b.store.Services().List(ctx)
	Expect(err).NotTo(HaveOccurred())
	sets, err := b.store.PermissionSets().List(ctx, id.ServiceID{})
	Expect(err).NotTo(HaveOccurred())
	return [3]int{len(agents), len(services), len(sets)}
}

var _ = Describe("Labels and annotations on Agents, Services, and Permission Sets", func() {
	var broker *metadataBroker

	AfterEach(func() {
		if broker != nil {
			broker.close()
			broker = nil
		}
	})

	Context("when GitOps reconciles an Agent on PostgreSQL", Label("docker"), func() {
		BeforeEach(func() {
			broker = newPostgresMetadataBroker()
		})

		// Scenario US1.2 from specs/20261127-093439-object-labels-annotations/spec.md
		It("should treat a reconcile with reordered metadata keys as a no-op without write or audit", func() {
			selector := "/api/agents?external_id=" + url.QueryEscape("urn:example:ai-registry:agent:review-bot")
			created := broker.expectAdmin(http.MethodPut, selector, metadataAgentBody(broker.setID, fixtures.SentinelMetadataJSON()), http.StatusCreated)
			stored := broker.storedAgent(created.body["id"].(string))
			random := rand.New(rand.NewPCG(20261127, 93439))
			for range 100 {
				body := metadataAgentBody(broker.setID, helpers.ShuffledMetadataJSON(fixtures.SentinelLabels(), fixtures.SentinelAnnotations(), random))
				replay := broker.expectAdmin(http.MethodPut, selector, body, http.StatusOK)
				Expect(replay.body["updated_at"]).To(Equal(created.body["updated_at"]))
				Expect(replay.body["metadata"]).To(Equal(created.body["metadata"]))
			}
			Expect(broker.storedAgent(created.body["id"].(string)).UpdatedAt).To(Equal(stored.UpdatedAt))
			Expect(broker.auditRecords("agents.agent.updated")).To(BeZero())
			broker.expectNoSentinelLeaks()
		})
	})

	Context("with in-memory storage", func() {
		BeforeEach(func() {
			broker = newMemoryMetadataBroker()
		})

		Context("when an operator manages Agent metadata", func() {
			// Scenario US1.1 from specs/20261127-093439-object-labels-annotations/spec.md
			It("should return submitted labels and annotations on create, read, and list", func() {
				random := rand.New(rand.NewPCG(1, 1))
				created := broker.expectAdmin(http.MethodPost, "/api/agents", metadataAgentBody(broker.setID, helpers.ShuffledMetadataJSON(fixtures.SentinelLabels(), fixtures.SentinelAnnotations(), random)), http.StatusCreated)
				want := map[string]any{"labels": stringMap(fixtures.SentinelLabels()), "annotations": stringMap(fixtures.SentinelAnnotations())}
				Expect(created.body["metadata"]).To(Equal(want))
				Expect(created.raw).To(ContainSubstring(`"labels":{"deployment":"local","sentinel.example.com/zz-label-7f3a9c":"zz-label-value-7f3a9c","team.example.com/owner":"platform"}`))
				read := broker.expectAdmin(http.MethodGet, "/api/agents/"+created.body["id"].(string), "", http.StatusOK)
				Expect(read.body["metadata"]).To(Equal(want))
				list := broker.expectAdmin(http.MethodGet, "/api/agents", "", http.StatusOK)
				Expect(list.raw).To(ContainSubstring(`"metadata":{"labels":{"deployment":"local",`))
				broker.expectNoSentinelLeaks()
			})

			// Scenario US1.3 from specs/20261127-093439-object-labels-annotations/spec.md
			It("should update and audit a label-only change without metadata in the audit record", func() {
				selector := "/api/agents?external_id=review-bot"
				created := broker.expectAdmin(http.MethodPut, selector, metadataAgentBody(broker.setID, fixtures.SentinelMetadataJSON()), http.StatusCreated)
				before := broker.storedAgent(created.body["id"].(string)).UpdatedAt
				labels := fixtures.SentinelLabels()
				labels["deployment"] = "production"
				broker.telemetry.Logs.Reset()
				updated := broker.expectAdmin(http.MethodPut, selector, metadataAgentBody(broker.setID, fixtures.MetadataJSON(labels, fixtures.SentinelAnnotations())), http.StatusOK)
				Expect(updated.body["metadata"].(map[string]any)["labels"]).To(HaveKeyWithValue("deployment", "production"))
				Expect(broker.storedAgent(created.body["id"].(string)).UpdatedAt).To(BeTemporally(">", before))
				Expect(broker.auditRecords("agents.agent.updated")).To(Equal(1))
				broker.expectNoSentinelLeaks()
			})

			// Scenario US1.4 from specs/20261127-093439-object-labels-annotations/spec.md
			It("should store omitted or empty metadata maps as empty on every Agent write path", func() {
				full := fixtures.SentinelMetadataJSON()
				created := broker.expectAdmin(http.MethodPost, "/api/agents", metadataAgentBody(broker.setID, ""), http.StatusCreated)
				Expect(created.body).NotTo(HaveKey("metadata"))
				agentPath := "/api/agents/" + created.body["id"].(string)
				broker.expectAdmin(http.MethodPut, agentPath, metadataAgentBody(broker.setID, full), http.StatusOK)
				cleared := broker.expectAdmin(http.MethodPut, agentPath, metadataAgentBody(broker.setID, `{}`), http.StatusOK)
				Expect(cleared.body).NotTo(HaveKey("metadata"))
				Expect(broker.storedAgent(created.body["id"].(string)).ObjectMetadata.IsEmpty()).To(BeTrue())
				selector := "/api/agents?external_id=review-bot"
				broker.expectAdmin(http.MethodPut, selector, metadataAgentBody(broker.setID, full), http.StatusCreated)
				labelsOnly := broker.expectAdmin(http.MethodPut, selector, metadataAgentBody(broker.setID, `{"labels":{"tier":"gold"}}`), http.StatusOK)
				Expect(labelsOnly.body["metadata"]).To(Equal(map[string]any{"labels": map[string]any{"tier": "gold"}}))
				omitted := broker.expectAdmin(http.MethodPut, selector, metadataAgentBody(broker.setID, ""), http.StatusOK)
				Expect(omitted.body).NotTo(HaveKey("metadata"))
				Expect(broker.storedAgent(omitted.body["id"].(string)).ObjectMetadata.IsEmpty()).To(BeTrue())
			})

			// Scenario US1.5 from specs/20261127-093439-object-labels-annotations/spec.md
			It("should reject null metadata labels or annotations on every Agent write path without change", func() {
				selector := "/api/agents?external_id=review-bot"
				created := broker.expectAdmin(http.MethodPut, selector, metadataAgentBody(broker.setID, fixtures.SentinelMetadataJSON()), http.StatusCreated)
				before := broker.storedAgent(created.body["id"].(string))
				paths := map[string]string{http.MethodPost + " /api/agents": "/api/agents", http.MethodPut + " id": "/api/agents/" + created.body["id"].(string), http.MethodPut + " selector": selector}
				for name, path := range paths {
					method := strings.Fields(name)[0]
					for metadata, message := range map[string]string{`null`: "metadata must not be null", `{"labels":null}`: "metadata.labels must not be null", `{"annotations":null}`: "metadata.annotations must not be null"} {
						rejected := broker.expectAdmin(method, path, metadataAgentBody(broker.setID, metadata), http.StatusBadRequest)
						Expect(rejected.body).To(Equal(map[string]any{"error": "validation failed", "message": message}), name)
					}
				}
				after := broker.storedAgent(created.body["id"].(string))
				Expect(after.UpdatedAt).To(Equal(before.UpdatedAt))
				Expect(after.ObjectMetadata.Equal(before.ObjectMetadata)).To(BeTrue())
				Expect(broker.resourceCounts()[0]).To(Equal(2), "no Agent was created")
			})
		})

		Context("when an operator submits invalid metadata", func() {
			// Scenario US2.1 from specs/20261127-093439-object-labels-annotations/spec.md
			It("should reject invalid label keys and values on all three resources and store nothing", func() {
				before := broker.resourceCounts()
				invalid := map[string]string{
					`{"labels":{" deployment":"local"}}`:                          "metadata.labels contains an invalid key",
					`{"labels":{"Team.example.com/owner":"platform"}}`:            "metadata.labels contains an invalid key",
					`{"labels":{"a/b/c":"x"}}`:                                    "metadata.labels contains an invalid key",
					`{"labels":{"deployment":"local "}}`:                          "metadata.labels contains an invalid value",
					`{"labels":{"deployment":"` + strings.Repeat("v", 64) + `"}}`: "metadata.labels contains an invalid value",
					`{"labels":{"deployment":"a","deployment":"b"}}`:              "metadata.labels contains a duplicate key",
				}
				for resource, create := range broker.metadataCreators() {
					for metadata, message := range invalid {
						reply := create(metadata)
						Expect(reply.status).To(Equal(http.StatusBadRequest), resource+" "+metadata)
						Expect(reply.body).To(Equal(map[string]any{"error": "validation failed", "message": message}), resource+" "+metadata)
					}
				}
				Expect(broker.resourceCounts()).To(Equal(before))
			})

			// Scenario US2.2 from specs/20261127-093439-object-labels-annotations/spec.md
			It("should reject reserved-prefix keys and accept look-alike prefixes on all three resources", func() {
				for resource, create := range broker.metadataCreators() {
					for _, metadata := range []string{
						`{"labels":{"agenticidentitybroker.dev/tier":"gold"}}`,
						`{"annotations":{"policy.agenticidentitybroker.dev/note":"x"}}`,
					} {
						reply := create(metadata)
						Expect(reply.status).To(Equal(http.StatusBadRequest), resource)
						Expect(reply.body["message"]).To(HaveSuffix("contains a reserved key"), resource)
					}
					accepted := create(`{"labels":{"example.agenticidentitybroker.dev.evil.com/tier":"gold"}}`)
					Expect(accepted.status).To(Equal(http.StatusCreated), resource+" "+accepted.raw)
					Expect(accepted.body["metadata"]).To(Equal(map[string]any{"labels": map[string]any{"example.agenticidentitybroker.dev.evil.com/tier": "gold"}}), resource)
				}
			})

			// Scenario US2.3 from specs/20261127-093439-object-labels-annotations/spec.md
			It("should accept 64 labels and 65536 annotation bytes and reject one more", func() {
				selector := "/api/agents?external_id=boundaries"
				created := broker.expectAdmin(http.MethodPut, selector, metadataAgentBody(broker.setID, ""), http.StatusCreated)
				paths := []string{selector, "/api/agents/" + created.body["id"].(string)}
				Expect(fixtures.DecodedMetadataSize(fixtures.AnnotationsOfDecodedSize(65536))).To(Equal(65536))
				Expect(fixtures.DecodedMetadataSize(fixtures.EscapedAnnotationsOfDecodedSize(65536))).To(Equal(65536))
				for _, path := range paths {
					for _, accepted := range []string{
						fixtures.MetadataJSON(fixtures.LabelsOfCount(64), nil),
						fixtures.MetadataJSON(nil, fixtures.AnnotationsOfDecodedSize(65536)),
						fixtures.MetadataJSON(fixtures.LabelsOfCount(64), fixtures.EscapedAnnotationsOfDecodedSize(65536)),
					} {
						broker.expectAdmin(http.MethodPut, path, metadataAgentBody(broker.setID, accepted), http.StatusOK)
					}
					for rejected, message := range map[string]string{
						fixtures.MetadataJSON(fixtures.LabelsOfCount(65), nil):                      "metadata.labels contains more than 64 entries",
						fixtures.MetadataJSON(nil, fixtures.AnnotationsOfDecodedSize(65537)):        "metadata.annotations exceeds 65536 bytes",
						fixtures.MetadataJSON(nil, fixtures.EscapedAnnotationsOfDecodedSize(65537)): "metadata.annotations exceeds 65536 bytes",
					} {
						Expect(broker.expectAdmin(http.MethodPut, path, metadataAgentBody(broker.setID, rejected), http.StatusBadRequest).body["message"]).To(Equal(message))
					}
				}
				stored := broker.storedAgent(created.body["id"].(string)).ObjectMetadata
				Expect(stored.Labels()).To(HaveLen(64))
				Expect(fixtures.DecodedMetadataSize(stored.Annotations())).To(Equal(65536))
			})

			// Scenario US2.4 from specs/20261127-093439-object-labels-annotations/spec.md
			It("should name the map and rule without echoing the submitted key or value", func() {
				for resource, create := range broker.metadataCreators() {
					for metadata, message := range map[string]string{
						`{"labels":{"` + fixtures.MetadataSentinelLabelKey + `":"` + fixtures.MetadataSentinelLabelValue + ` "}}`: "metadata.labels contains an invalid value",
						`{"annotations":{"agenticidentitybroker.dev/` + fixtures.MetadataSentinelLabelValue + `":"x"}}`:           "metadata.annotations contains a reserved key",
					} {
						reply := create(metadata)
						Expect(reply.body).To(Equal(map[string]any{"error": "validation failed", "message": message}), resource)
						for _, sentinel := range fixtures.MetadataSentinels() {
							Expect(reply.raw).NotTo(ContainSubstring(sentinel), resource)
						}
					}
				}
				broker.expectNoSentinelLeaks()
			})

			// Scenario US2.5 from specs/20261127-093439-object-labels-annotations/spec.md
			It("should keep keys that differ only in case distinct and exactly as sent", func() {
				for resource, create := range broker.metadataCreators() {
					reply := create(`{"labels":{"exposure":"internal","Exposure":"public"}}`)
					Expect(reply.status).To(Equal(http.StatusCreated), resource)
					Expect(reply.raw).To(ContainSubstring(`"labels":{"Exposure":"public","exposure":"internal"}`), resource)
				}
			})
		})

		Context("when an operator manages Permission Set and Service metadata", func() {
			// Scenario US3.1 from specs/20261127-093439-object-labels-annotations/spec.md
			It("should return Permission Set labels and annotations on create, read, and list", func() {
				created := broker.expectAdmin(http.MethodPost, "/api/permission-sets", metadataPermissionSetBody("Repository read", broker.serviceID, fixtures.SentinelMetadataJSON()), http.StatusCreated)
				want := map[string]any{"labels": stringMap(fixtures.SentinelLabels()), "annotations": stringMap(fixtures.SentinelAnnotations())}
				Expect(created.body["metadata"]).To(Equal(want))
				read := broker.expectAdmin(http.MethodGet, "/api/permission-sets/"+created.body["id"].(string), "", http.StatusOK)
				Expect(read.body["metadata"]).To(Equal(want))
				list := broker.expectAdmin(http.MethodGet, "/api/permission-sets", "", http.StatusOK)
				Expect(list.raw).To(ContainSubstring(`"metadata":{"labels":{"deployment":"local",`))
				broker.expectNoSentinelLeaks()
			})

			// Scenario US3.2 from specs/20261127-093439-object-labels-annotations/spec.md
			It("should clear Permission Set metadata on a PUT that omits it and advance updated_at", func() {
				created := broker.expectAdmin(http.MethodPost, "/api/permission-sets", metadataPermissionSetBody("Repository read", broker.serviceID, fixtures.SentinelMetadataJSON()), http.StatusCreated)
				setID := id.MustParsePermissionSetID(created.body["id"].(string))
				before, err := broker.store.PermissionSets().Get(context.Background(), setID)
				Expect(err).NotTo(HaveOccurred())
				broker.telemetry.Logs.Reset()
				updated := broker.expectAdmin(http.MethodPut, "/api/permission-sets/"+setID.String(), metadataPermissionSetBody("Repository read", broker.serviceID, ""), http.StatusOK)
				Expect(updated.body).NotTo(HaveKey("metadata"))
				after, err := broker.store.PermissionSets().Get(context.Background(), setID)
				Expect(err).NotTo(HaveOccurred())
				Expect(after.ObjectMetadata.IsEmpty()).To(BeTrue())
				Expect(after.UpdatedAt).To(BeTemporally(">", before.UpdatedAt))
				Expect(broker.auditRecords("permissionset.permission_set.updated")).To(Equal(1))
				broker.expectNoSentinelLeaks()
			})

			// Scenario US3.3 from specs/20261127-093439-object-labels-annotations/spec.md
			It("should preserve Service metadata on an update that omits it", func() {
				created := broker.expectAdmin(http.MethodPost, "/api/services", metadataServiceBody("Atlas", fixtures.SentinelMetadataJSON()), http.StatusCreated)
				path := "/api/services/" + created.body["id"].(string)
				updated := broker.expectAdmin(http.MethodPut, path, metadataServiceBody("Atlas renamed", ""), http.StatusOK)
				Expect(updated.body["display_name"]).To(Equal("Atlas renamed"))
				Expect(updated.body["metadata"]).To(Equal(created.body["metadata"]))
				Expect(broker.expectAdmin(http.MethodGet, path, "", http.StatusOK).body["metadata"]).To(Equal(created.body["metadata"]))
				broker.expectNoSentinelLeaks()
			})

			// Scenario US3.4 from specs/20261127-093439-object-labels-annotations/spec.md
			It("should replace whole Service metadata, clear it with {}, and reject null", func() {
				created := broker.expectAdmin(http.MethodPost, "/api/services", metadataServiceBody("Atlas", fixtures.SentinelMetadataJSON()), http.StatusCreated)
				path := "/api/services/" + created.body["id"].(string)
				replaced := broker.expectAdmin(http.MethodPut, path, metadataServiceBody("Atlas", `{"labels":{"tier":"gold"}}`), http.StatusOK)
				Expect(replaced.body["metadata"]).To(Equal(map[string]any{"labels": map[string]any{"tier": "gold"}}))
				rejected := broker.expectAdmin(http.MethodPut, path, metadataServiceBody("Atlas renamed", `null`), http.StatusBadRequest)
				Expect(rejected.body).To(Equal(map[string]any{"error": "validation failed", "message": "metadata must not be null"}))
				unchanged := broker.expectAdmin(http.MethodGet, path, "", http.StatusOK)
				Expect(unchanged.body["display_name"]).To(Equal("Atlas"))
				Expect(unchanged.body["metadata"]).To(Equal(replaced.body["metadata"]))
				cleared := broker.expectAdmin(http.MethodPut, path, metadataServiceBody("Atlas", `{}`), http.StatusOK)
				Expect(cleared.body).NotTo(HaveKey("metadata"))
				Expect(broker.expectAdmin(http.MethodGet, path, "", http.StatusOK).body).NotTo(HaveKey("metadata"))
			})
		})
	})

	Context("when the involved resources carry metadata at runtime", func() {
		var annotated *metadataBroker

		BeforeEach(func() {
			broker = newMemoryMetadataBroker()
			annotated = newMemoryMetadataBroker()
			annotated.annotate()
		})

		AfterEach(func() {
			if annotated != nil {
				annotated.close()
				annotated = nil
			}
		})

		// Scenario US3.5 from specs/20261127-093439-object-labels-annotations/spec.md
		It("should leave consent, token claims, approvals, and end-user responses unchanged by metadata", func() {
			plain := broker.runtimeJourney()
			withMetadata := annotated.runtimeJourney()
			Expect(plain).To(SatisfyAll(
				HaveKeyWithValue("consent detail", HaveKeyWithValue("status", http.StatusOK)),
				HaveKeyWithValue("consent decision", HaveKeyWithValue("status", BeElementOf(http.StatusOK, http.StatusCreated))),
				HaveKeyWithValue("token exchange", HaveKeyWithValue("status", http.StatusOK)),
				HaveKeyWithValue("approval", HaveKeyWithValue("status", http.StatusCreated)),
				HaveKeyWithValue("approval repeat", HaveKeyWithValue("status", http.StatusOK)),
			))
			Expect(withMetadata).To(Equal(plain))
			for _, b := range []*metadataBroker{broker, annotated} {
				for _, raw := range b.endUserBodies {
					Expect(raw).NotTo(ContainSubstring(`"metadata"`))
					for _, sentinel := range fixtures.MetadataSentinels() {
						Expect(raw).NotTo(ContainSubstring(sentinel))
					}
				}
				b.expectNoSentinelLeaks()
			}
			Expect(broker.adminMetadata()).To(BeEmpty(), "end-user input wrote no metadata")
			Expect(annotated.adminMetadata()).To(HaveLen(3))
			for _, metadata := range annotated.adminMetadata() {
				Expect(metadata).To(Equal(map[string]any{"labels": stringMap(fixtures.SentinelLabels()), "annotations": stringMap(fixtures.SentinelAnnotations())}))
			}
		})
	})
})

func stringMap(source map[string]string) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// annotate stores sentinel metadata on the Agent, Service, and Permission Set
// that the runtime journey uses.
func (b *metadataBroker) annotate() {
	ctx := context.Background()
	metadata, err := model.NewObjectMetadata(fixtures.SentinelLabels(), fixtures.SentinelAnnotations())
	Expect(err).NotTo(HaveOccurred())
	agent, err := b.store.Agents().Get(ctx, b.agent.ID)
	Expect(err).NotTo(HaveOccurred())
	agent.ObjectMetadata = metadata
	Expect(b.store.Agents().Update(ctx, agent)).To(Succeed())
	service, err := b.store.Services().Get(ctx, b.serviceID)
	Expect(err).NotTo(HaveOccurred())
	service.ObjectMetadata, service.ObjectMetadataProvided = metadata, true
	Expect(b.store.Services().Update(ctx, service, nil)).To(Succeed())
	set, err := b.store.PermissionSets().Get(ctx, b.setID)
	Expect(err).NotTo(HaveOccurred())
	set.ObjectMetadata = metadata
	Expect(b.store.PermissionSets().Update(ctx, set)).To(Succeed())
}

// adminMetadata reads the metadata of the journey's resources through the Admin API.
func (b *metadataBroker) adminMetadata() []any {
	var found []any
	for _, path := range []string{"/api/agents/" + b.agent.ID.String(), "/api/services/" + b.serviceID.String(), "/api/permission-sets/" + b.setID.String()} {
		if metadata, ok := b.expectAdmin(http.MethodGet, path, "", http.StatusOK).body["metadata"]; ok {
			found = append(found, metadata)
		}
	}
	return found
}

// runtimeJourney consents, exchanges a token, and requests a tool approval,
// sending metadata through End-User inputs, and returns every outcome with
// run-specific identifiers and times removed.
func (b *metadataBroker) runtimeJourney() map[string]any {
	principal := fixtures.DefaultPrincipal().String()
	Expect(b.store.UserSessions().Create(context.Background(), fixtures.GitHubSessionForPrincipal(principal))).To(Succeed())
	read := func(method, path, body string, headers map[string]string) any {
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		response, err := b.enduser.DirectRequest(method, path, principal, headers, reader)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = response.Body.Close() }()
		raw, err := io.ReadAll(response.Body)
		Expect(err).NotTo(HaveOccurred())
		b.endUserBodies = append(b.endUserBodies, string(raw))
		var value any
		Expect(json.Unmarshal(raw, &value)).To(Succeed(), string(raw))
		return map[string]any{"status": response.StatusCode, "body": b.normalize(value)}
	}
	jsonHeaders := map[string]string{"Content-Type": "application/json"}
	outcome := map[string]any{}
	outcome["consent list"] = read(http.MethodGet, "/api/consent/agents", "", nil)
	outcome["consent detail"] = read(http.MethodGet, "/api/consent/agents/"+b.agent.ID.String(), "", nil)
	grant := `{"granted_permission_sets":{"` + b.setID.String() + `":["` + b.serviceID.String() + `"]},"metadata":` + fixtures.SentinelMetadataJSON() + `}`
	outcome["consent decision"] = read(http.MethodPost, "/api/consent/agents/"+b.agent.ID.String()+"/grants", grant, jsonHeaders)
	outcome["grants"] = read(http.MethodGet, "/api/consent/agents/"+b.agent.ID.String()+"/grants", "", nil)
	form := proactiveExchangeForm(b.upstream, principal, b.agent)
	form.Set("metadata", fixtures.SentinelMetadataJSON())
	outcome["token exchange"] = read(http.MethodPost, "/oauth2/token", form.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	auth, err := helpers.NewApprovalRequestAuthFixture(b.upstream, b.agent.ID)
	Expect(err).NotTo(HaveOccurred())
	approval := helpers.CreateApprovalRequest{ToolName: "create_issue", Arguments: map[string]any{"repo": "acme/agents"}, Metadata: helpers.CreateApprovalMetadata{Description: "Create an issue"}}
	encoded, err := json.Marshal(approval)
	Expect(err).NotTo(HaveOccurred())
	headers := helpers.ApprovalCreateHeaders(subjectTokenFor(auth, principal), auth.ClientAssertion, auth.AgentID)
	headers["Content-Type"] = "application/json"
	outcome["approval"] = read(http.MethodPost, "/api/approvals", string(encoded), headers)
	outcome["approval repeat"] = read(http.MethodPost, "/api/approvals", string(encoded), headers)
	return outcome
}

var runtimeGeneratedID = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

var runtimeVolatileKeys = map[string]bool{
	"id": true, "created_at": true, "updated_at": true, "createdAt": true, "updatedAt": true,
	"expires_at": true, "expiresAt": true, "valid_until": true, "validUntil": true,
	"approval_id": true, "approvalId": true, "grant_id": true, "grantId": true,
	"lastUsedAt": true, "last_used_at": true, "connectedAt": true, "connected_at": true,
}

func (b *metadataBroker) normalize(value any) any {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			if runtimeVolatileKeys[key] {
				result[key] = "<volatile>"
				continue
			}
			result[key] = b.normalize(item)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for i, item := range value {
			result[i] = b.normalize(item)
		}
		return result
	case string:
		if _, err := time.Parse(time.RFC3339, value); err == nil {
			return "<time>"
		}
		value = strings.ReplaceAll(value, b.agent.ID.String(), "<agent>")
		return runtimeGeneratedID.ReplaceAllStringFunc(value, func(match string) string {
			if match == b.serviceID.String() || match == b.setID.String() {
				return match
			}
			return "<generated>"
		})
	default:
		return value
	}
}
