// Package extproc_test contains E2E acceptance tests for OPA-based authorization in ExtProc.
//
// This file covers US2–US5 acceptance scenarios from specs/020-extproc-opa-authorization/spec.md.
// Tests run in-process using the bootstrap harness (no Docker required).
//
// Each It() block maps 1:1 to one acceptance scenario.
//
// Scenario mapping:
//   - US2 Scenario 1–3: Protocol-aware input extraction (mcp_tool_call, unknown, mcp_method)
//   - US3 Scenario 1–3: Local Rego file loading and startup validation
//   - US4 Scenario 1–2: OPA config file / bundle pulling
//   - US5 Scenario 1–2: OPA disabled (backward compatibility)
//   - Edge cases: mutual exclusivity, evaluation timeout
//
// Focused run:
//
//	cd tests/e2e/extproc && go test -list "OPA" ./...
package extproc_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	sdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"google.golang.org/grpc"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
)

// opaLogger is the structured logger for OPA authorization tests.
var opaLogger = bootstrap.NewTestLogger()

// policyDir returns the absolute path to the test policy fixtures directory.
func policyDir() string {
	dir, err := filepath.Abs("fixtures/policies")
	if err != nil {
		panic("cannot resolve fixtures/policies: " + err.Error())
	}
	return dir
}

// policyPath returns the absolute path to a named Rego policy fixture.
func policyPath(name string) string {
	return filepath.Join(policyDir(), name)
}

// opaEnabledConfig returns a base config with OPA authorization enabled.
// The policy source must be set by the caller before use.
func opaEnabledConfig(policyFile string) *extprocconfig.Config {
	cfg := fixtures.DefaultConfig()
	cfg.Authorization = extprocconfig.AuthorizationConfig{
		Enabled: true,
		Policy: extprocconfig.PolicyConfig{
			Path:     policyFile,
			Package:  "aib.extproc.authz",
			Decision: "result",
		},
		DefaultDecision:   "deny",
		EvaluationTimeout: 500 * time.Millisecond,
		MaxBodySize:       1024 * 1024,
	}
	return cfg
}

// opaDisabledConfig returns a base config with OPA authorization explicitly disabled.
func opaDisabledConfig() *extprocconfig.Config {
	cfg := fixtures.DefaultConfig()
	cfg.Authorization = extprocconfig.AuthorizationConfig{
		Enabled: false,
	}
	return cfg
}

// toolCallBody returns a minimal MCP JSON-RPC tools/call body for the given tool name.
func toolCallBody(toolName string) []byte {
	return []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + toolName + `","arguments":{"repo":"acme/app"}}}`)
}

// initializeBody returns a minimal MCP JSON-RPC initialize request body.
func initializeBody() []byte {
	return []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"test-agent","version":"1.0"}}}`)
}

// unknownProtocolBody returns an arbitrary payload for unrecognized protocol tests.
func unknownProtocolBody() []byte {
	return []byte(`{"action":"run","tool":"bash","command":"ls -la"}`)
}

var _ = Describe("OPA Authorization", func() {

	// -------------------------------------------------------------------------
	// User Story 2: Protocol-Aware Input Extraction (P2)
	// specs/020-extproc-opa-authorization/spec.md — US2
	// -------------------------------------------------------------------------
	Describe("US2: Protocol-Aware Input Extraction", func() {
		var (
			env        *bootstrap.TestEnvironment
			grpcClient extprocv3.ExternalProcessorClient
			conn       *grpc.ClientConn
		)

		BeforeEach(func() {
			cfg := opaEnabledConfig(policyPath("assert_us2_input_shapes.rego"))
			env = bootstrap.NewTestEnvironment(cfg, opaLogger)
			env.Start()

			env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken)
			expiresIn := fixtures.StandardExpiresIn
			env.MockTokenExchange.WithExpiresIn(&expiresIn)

			grpcClient, conn = env.NewExtProcClient()
		})

		AfterEach(func() {
			conn.Close() //nolint:errcheck
			env.Stop()
		})

		// Scenario US2.1 from specs/020-extproc-opa-authorization/spec.md
		// Given an MCP tools/call request with tool name "create_issue" and arguments,
		// When OPA evaluates the request,
		// Then input contains type="mcp_tool_call", mcp.tool_name="create_issue".
		It("should build OPA input with mcp.tool_name for tools/call", func() {
			headersReq := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				WithHeader(":method", "POST").
				WithAgentgatewayProtocol("mcp").
				BuildWithMetadata()

			body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_issue","arguments":{"title":"Bug","repo":"acme/app"}}}`)

			headersResp, bodyResp := helpers.SendHeadersAndBody(context.Background(), grpcClient, headersReq, body)

			Expect(headersResp).NotTo(BeNil())
			expectedAuth := "Bearer " + fixtures.FreshExchangedToken
			Expect(headersResp).To(helpers.HaveReplacedAuthorizationHeader(expectedAuth),
				"headers phase should perform eager token exchange before OPA evaluates the body")
			Expect(bodyResp).NotTo(BeNil())
			Expect(bodyResp).To(helpers.BeForwardedRequestBody(),
				"correct MCP input should be allowed in the body phase")
		})

		// Scenario US2.2 from specs/020-extproc-opa-authorization/spec.md
		// Given a request with an unrecognized protocol in metadata,
		// When OPA evaluates the request,
		// Then type="unknown" and raw body is available in input.attributes.request.http.body.
		It("should set type=unknown for unrecognized protocol", func() {
			headersReq := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				WithAgentgatewayProtocol("some-unknown-protocol").
				BuildWithMetadata()

			headersResp, bodyResp := helpers.SendHeadersAndBody(context.Background(), grpcClient, headersReq, unknownProtocolBody())

			Expect(headersResp).NotTo(BeNil())
			Expect(headersResp).To(helpers.HaveReplacedAuthorizationHeader("Bearer "+fixtures.FreshExchangedToken),
				"headers phase should perform eager token exchange before OPA evaluates the body")
			Expect(bodyResp).NotTo(BeNil())
			Expect(bodyResp).To(helpers.BeForwardedRequestBody(),
				"unknown-protocol requests should still be allowed in the body phase when the policy permits them")
		})

		// Scenario US2.3 from specs/020-extproc-opa-authorization/spec.md
		// Given an MCP initialize request,
		// When OPA evaluates the request,
		// Then input contains type="mcp_method", mcp.method="initialize".
		It("should build OPA input with mcp.method=initialize", func() {
			headersReq := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				WithHeader(":method", "POST").
				WithAgentgatewayProtocol("mcp").
				BuildWithMetadata()

			headersResp, bodyResp := helpers.SendHeadersAndBody(context.Background(), grpcClient, headersReq, initializeBody())

			Expect(headersResp).NotTo(BeNil())
			Expect(headersResp).To(helpers.HaveReplacedAuthorizationHeader("Bearer "+fixtures.FreshExchangedToken),
				"headers phase should perform eager token exchange before OPA evaluates the body")
			Expect(bodyResp).NotTo(BeNil())
			Expect(bodyResp).To(helpers.BeForwardedRequestBody(),
				"initialize requests should be allowed in the body phase when the input shape matches policy expectations")
		})
	})

	// -------------------------------------------------------------------------
	// Spec 044: MCP Target Server Name in OPA Authorization Input
	// specs/044-extproc-opa-mcp-target-server/spec.md — US1, US2, US3
	// -------------------------------------------------------------------------
	Describe("Spec 044: MCP Target Server Name", func() {

		Context("US1/US2: input.mcp.target_server_name shape across request types", func() {
			var (
				env        *bootstrap.TestEnvironment
				grpcClient extprocv3.ExternalProcessorClient
				conn       *grpc.ClientConn
			)

			BeforeEach(func() {
				cfg := opaEnabledConfig(policyPath("assert_target_server_name_input.rego"))
				env = bootstrap.NewTestEnvironment(cfg, opaLogger)
				env.Start()

				env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken)
				expiresIn := fixtures.StandardExpiresIn
				env.MockTokenExchange.WithExpiresIn(&expiresIn)

				grpcClient, conn = env.NewExtProcClient()
			})

			AfterEach(func() {
				conn.Close() //nolint:errcheck
				env.Stop()
			})

			// Scenario US1.1 from specs/044-extproc-opa-mcp-target-server/spec.md
			// Given an MCP tools/call request with agentgateway mcp_server="github-mcp",
			// When OPA evaluates the request,
			// Then input.mcp.target_server_name == "github-mcp".
			It("should build OPA input with mcp.target_server_name for tools/call", func() {
				headersReq := helpers.NewRequestHeaders().
					WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
					WithHeader(":method", "POST").
					WithAgentgatewayProtocol("mcp").
					WithAgentgatewayMCPServer("github-mcp").
					BuildWithMetadata()

				headersResp, bodyResp := helpers.SendHeadersAndBody(context.Background(), grpcClient, headersReq,
					toolCallBody("list_repositories"))

				Expect(headersResp).NotTo(BeNil())
				Expect(headersResp).To(helpers.HaveReplacedAuthorizationHeader("Bearer "+fixtures.FreshExchangedToken),
					"headers phase should perform eager token exchange before OPA evaluates the body")
				Expect(bodyResp).NotTo(BeNil())
				Expect(bodyResp).To(helpers.BeForwardedRequestBody(),
					"tools/call with matching target_server_name should be allowed in the body phase")
			})

			// Scenario US1.1 (mcp_method variant) from specs/044-extproc-opa-mcp-target-server/spec.md
			// Given an MCP initialize request with agentgateway mcp_server="github-mcp",
			// When OPA evaluates the request,
			// Then input.mcp.target_server_name == "github-mcp" alongside input.mcp.method.
			It("should build OPA input with mcp.target_server_name for non-tool-call methods", func() {
				headersReq := helpers.NewRequestHeaders().
					WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
					WithHeader(":method", "POST").
					WithAgentgatewayProtocol("mcp").
					WithAgentgatewayMCPServer("github-mcp").
					BuildWithMetadata()

				headersResp, bodyResp := helpers.SendHeadersAndBody(context.Background(), grpcClient, headersReq, initializeBody())

				Expect(headersResp).NotTo(BeNil())
				Expect(headersResp).To(helpers.HaveReplacedAuthorizationHeader("Bearer "+fixtures.FreshExchangedToken),
					"headers phase should perform eager token exchange before OPA evaluates the body")
				Expect(bodyResp).NotTo(BeNil())
				Expect(bodyResp).To(helpers.BeForwardedRequestBody(),
					"initialize with matching target_server_name should be allowed in the body phase")
			})

			// Scenario US2.1 from specs/044-extproc-opa-mcp-target-server/spec.md
			// Given a header-only MCP request (end_of_stream=true) with agentgateway
			// mcp_server="github-mcp", When OPA evaluates the mcp_headers_only input,
			// Then input.mcp.target_server_name == "github-mcp".
			It("should build OPA input with mcp.target_server_name for headers-only requests", func() {
				headersReq := helpers.NewRequestHeaders().
					WithHeader(":method", "GET").
					WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
					WithAgentgatewayProtocol("mcp").
					WithAgentgatewayMCPServer("github-mcp").
					WithEndOfStream(true).
					BuildWithMetadata()

				resp := helpers.SendRequestHeaders(context.Background(), grpcClient, headersReq)

				Expect(resp).NotTo(BeNil())
				Expect(resp).To(helpers.HaveReplacedAuthorizationHeader("Bearer "+fixtures.FreshExchangedToken),
					"mcp_headers_only request with matching target_server_name should be allowed and token-exchanged")
			})

			// Scenario US3.1 from specs/044-extproc-opa-mcp-target-server/spec.md (FR-004)
			// Given an MCP tools/call request where agentgateway sends no mcp_server metadata,
			// When ExtProc evaluates the request headers,
			// Then the request is rejected with 403 before OPA is ever invoked (misconfiguration),
			// mirroring how absent protocol metadata is rejected.
			It("should reject with 403 when agentgateway sends no mcp_server metadata", func() {
				headersReq := helpers.NewRequestHeaders().
					WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
					WithHeader(":method", "POST").
					WithAgentgatewayProtocol("mcp").
					WithoutAgentgatewayMCPServer().
					BuildWithMetadata()

				headersResp, bodyResp := helpers.SendHeadersAndBody(context.Background(), grpcClient, headersReq,
					toolCallBody("list_repositories"))

				Expect(headersResp).NotTo(BeNil())
				Expect(headersResp).To(helpers.HaveImmediateResponseWithStatus(403),
					"absent target_server_name metadata must be rejected at the headers phase (FR-004), mirroring absent protocol metadata")
				Expect(bodyResp).To(BeNil(), "no body phase should occur once the headers phase rejects the request")
			})
		})

		Context("US1: per-MCP-server policy scoping", func() {
			var (
				env        *bootstrap.TestEnvironment
				grpcClient extprocv3.ExternalProcessorClient
				conn       *grpc.ClientConn
			)

			BeforeEach(func() {
				cfg := opaEnabledConfig(policyPath("allow_by_target_server_name.rego"))
				env = bootstrap.NewTestEnvironment(cfg, opaLogger)
				env.Start()

				env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken)
				expiresIn := fixtures.StandardExpiresIn
				env.MockTokenExchange.WithExpiresIn(&expiresIn)

				grpcClient, conn = env.NewExtProcClient()
			})

			AfterEach(func() {
				conn.Close() //nolint:errcheck
				env.Stop()
			})

			// Scenario US1.1 from specs/044-extproc-opa-mcp-target-server/spec.md
			// Given a policy that allows tool calls only when target_server_name=="github-mcp",
			// When a tool call arrives with mcp_server="github-mcp",
			// Then the request is allowed.
			It("should allow the tool call when target_server_name matches the authorized server", func() {
				headersReq := helpers.NewRequestHeaders().
					WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
					WithHeader(":method", "POST").
					WithAgentgatewayProtocol("mcp").
					WithAgentgatewayMCPServer("github-mcp").
					BuildWithMetadata()

				_, bodyResp := helpers.SendHeadersAndBody(context.Background(), grpcClient, headersReq,
					toolCallBody("list_repositories"))

				Expect(bodyResp).NotTo(BeNil())
				Expect(bodyResp).To(helpers.BeForwardedRequestBody(),
					"tool call targeting the authorized MCP server should be allowed")
			})

			// Scenario US1.2 from specs/044-extproc-opa-mcp-target-server/spec.md
			// Given the same policy, When a tool call arrives with mcp_server="salesforce-mcp"
			// (a different, unauthorized server), Then the request is denied with 403.
			It("should deny the tool call when target_server_name targets a different server", func() {
				headersReq := helpers.NewRequestHeaders().
					WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
					WithHeader(":method", "POST").
					WithAgentgatewayProtocol("mcp").
					WithAgentgatewayMCPServer("salesforce-mcp").
					BuildWithMetadata()

				_, bodyResp := helpers.SendHeadersAndBody(context.Background(), grpcClient, headersReq,
					toolCallBody("list_repositories"))

				Expect(bodyResp).NotTo(BeNil())
				Expect(bodyResp).To(helpers.HaveImmediateResponseWithStatus(403),
					"tool call targeting an unauthorized MCP server should be denied")
			})

			// Edge case from specs/044-extproc-opa-mcp-target-server/spec.md
			// Given the same policy, When a tool call arrives with no mcp_server metadata at all,
			// Then the request is rejected with 403 at the headers phase by ExtProc's mandatory
			// mcp_server check (FR-004) — it never reaches the policy's own deny rule.
			It("should reject with 403 when target_server_name metadata is absent entirely", func() {
				headersReq := helpers.NewRequestHeaders().
					WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
					WithHeader(":method", "POST").
					WithAgentgatewayProtocol("mcp").
					WithoutAgentgatewayMCPServer().
					BuildWithMetadata()

				headersResp, bodyResp := helpers.SendHeadersAndBody(context.Background(), grpcClient, headersReq,
					toolCallBody("list_repositories"))

				Expect(headersResp).NotTo(BeNil())
				Expect(headersResp).To(helpers.HaveImmediateResponseWithStatus(403),
					"absent target_server_name metadata must be rejected before OPA is invoked (FR-004)")
				Expect(bodyResp).To(BeNil(), "no body phase should occur once the headers phase rejects the request")
			})
		})
	})

	// -------------------------------------------------------------------------
	// User Story 3: Local Rego File for Quick Setup (P3)
	// specs/020-extproc-opa-authorization/spec.md — US3
	// -------------------------------------------------------------------------
	Describe("US3: Local Rego File", func() {

		// Scenario US3.1 from specs/020-extproc-opa-authorization/spec.md
		// Given a valid Rego file at the configured path,
		// When ExtProc starts,
		// Then the policy is compiled and applied to requests.
		It("should compile and apply local Rego policy at startup", func() {
			// US3 Scenario 1 from specs/020-extproc-opa-authorization/spec.md

			cfg := opaEnabledConfig(policyPath("allow_readonly.rego"))
			env := bootstrap.NewTestEnvironment(cfg, opaLogger)
			env.Start()
			defer env.Stop()

			env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken)
			expiresIn := fixtures.StandardExpiresIn
			env.MockTokenExchange.WithExpiresIn(&expiresIn)

			client, conn := env.NewExtProcClient()
			defer conn.Close() //nolint:errcheck

			// Allowed tool: list_repositories
			allowedReq := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				WithHeader(":method", "POST").
				WithAgentgatewayProtocol("mcp").
				BuildWithMetadata()

			allowedHeadersResp, allowedBodyResp := helpers.SendHeadersAndBody(context.Background(), client, allowedReq,
				toolCallBody("list_repositories"))

			Expect(allowedHeadersResp).NotTo(BeNil())
			Expect(allowedHeadersResp).To(helpers.HaveReplacedAuthorizationHeader("Bearer "+fixtures.FreshExchangedToken),
				"headers phase should perform eager token exchange before OPA evaluates the body")
			Expect(allowedBodyResp).NotTo(BeNil())
			Expect(allowedBodyResp).To(helpers.BeForwardedRequestBody(),
				"list_repositories should be allowed in the body phase by allow_readonly policy")

			// Denied tool: delete_repository
			client2, conn2 := env.NewExtProcClient()
			defer conn2.Close() //nolint:errcheck

			deniedReq := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				WithHeader(":method", "POST").
				WithAgentgatewayProtocol("mcp").
				BuildWithMetadata()

			_, deniedBodyResp := helpers.SendHeadersAndBody(context.Background(), client2, deniedReq,
				toolCallBody("delete_repository"))

			Expect(deniedBodyResp).NotTo(BeNil())
			Expect(deniedBodyResp).To(helpers.HaveImmediateResponseWithStatus(403),
				"delete_repository should be denied by allow_readonly policy")
			Expect(deniedBodyResp).To(helpers.HaveImmediateResponseWithBody("tool is destructive"),
				"denial response should include the reason from the Rego deny rule")
		})

		// Scenario US3.2 from specs/020-extproc-opa-authorization/spec.md
		// Given a Rego file with syntax errors,
		// When ExtProc starts,
		// Then the service fails to start with a clear compilation error.
		It("should fail startup with clear error for invalid Rego", func() {
			// US3 Scenario 2 from specs/020-extproc-opa-authorization/spec.md

			// Write a temporary Rego file with a syntax error
			tmpDir := GinkgoT().TempDir()
			invalidRegoPath := filepath.Join(tmpDir, "invalid.rego")
			err := os.WriteFile(invalidRegoPath, []byte(`package aib.extproc.authz
this is not valid rego syntax !!!
`), 0600)
			Expect(err).NotTo(HaveOccurred())

			cfg := opaEnabledConfig(invalidRegoPath)

			// When: attempting to build a bootstrap environment with the invalid policy
			// The server should fail to start (NewTokenExchanger or bootstrap.Start() returns an error)
			err = bootstrap.StartWithOPAConfig(cfg, opaLogger)

			// Then: startup fails with a clear error referencing the file
			Expect(err).To(HaveOccurred(),
				"ExtProc should fail to start when Rego has syntax errors")
			Expect(err.Error()).To(ContainSubstring("invalid.rego"),
				"error message should reference the invalid Rego file path")
		})

		// Scenario US3.3 from specs/020-extproc-opa-authorization/spec.md
		// Given a configured Rego file path that does not exist,
		// When ExtProc starts,
		// Then the service fails to start indicating the file was not found.
		It("should fail startup when Rego file path does not exist", func() {
			// US3 Scenario 3 from specs/020-extproc-opa-authorization/spec.md

			cfg := opaEnabledConfig("/nonexistent/path/policy.rego")

			err := bootstrap.StartWithOPAConfig(cfg, opaLogger)

			Expect(err).To(HaveOccurred(),
				"ExtProc should fail to start when Rego file does not exist")
			Expect(err.Error()).To(Or(
				ContainSubstring("not found"),
				ContainSubstring("no such file"),
				ContainSubstring("/nonexistent/path/policy.rego"),
			), "error should indicate the file was not found")
		})
	})

	// -------------------------------------------------------------------------
	// User Story 4: OPA Config File for Bundle Pulling (P4)
	// specs/020-extproc-opa-authorization/spec.md — US4
	// -------------------------------------------------------------------------
	Describe("US4: OPA Config File", func() {

		// Scenario US4.1 from specs/020-extproc-opa-authorization/spec.md
		// Given a valid OPA config file with bundle server settings,
		// When ExtProc starts,
		// Then OPA initializes with the provided config.
		It("should initialize OPA from config file with bundle settings", func() {
			// US4 Scenario 1 from specs/020-extproc-opa-authorization/spec.md

			// Write a temporary OPA config that loads from a local file bundle directory.
			// Uses a dedicated bundle/ subdirectory containing only allow_readonly.rego to avoid
			// rule conflicts when all policy fixtures share the same package name.
			tmpDir := GinkgoT().TempDir()
			opaConfigPath := filepath.Join(tmpDir, "opa-config.yaml")
			bundleDir := filepath.Join(policyDir(), "bundle")
			opaConfigContent := "bundles:\n  local:\n    resource: \"file://" + bundleDir + "\"\n"
			err := os.WriteFile(opaConfigPath, []byte(opaConfigContent), 0600)
			Expect(err).NotTo(HaveOccurred())

			cfg := fixtures.DefaultConfig()
			cfg.Authorization = extprocconfig.AuthorizationConfig{
				Enabled: true,
				Policy: extprocconfig.PolicyConfig{
					ConfigFile: opaConfigPath,
					Package:    "aib.extproc.authz",
					Decision:   "result",
				},
				DefaultDecision:   "deny",
				EvaluationTimeout: 2 * time.Second, // longer timeout for bundle loading
				MaxBodySize:       1024 * 1024,
			}

			authorizer, err := bootstrap.NewOPAAuthorizer(cfg, opaLogger)
			Expect(err).NotTo(HaveOccurred(), "OPA initialisation should not fail")

			env := bootstrap.NewTestEnvironment(cfg, opaLogger)
			defer authorizer.Stop(context.Background()) //nolint:errcheck
			defer env.Stop()
			env.StartWithAuthorizer(authorizer)

			env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken)
			expiresIn := fixtures.StandardExpiresIn
			env.MockTokenExchange.WithExpiresIn(&expiresIn)

			// The allow_readonly.rego in the bundle allows list_repositories.
			headersReq := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				WithHeader(":method", "POST").
				WithAgentgatewayProtocol("mcp").
				BuildWithMetadata()

			Eventually(func() bool {
				client, conn := env.NewExtProcClient()
				defer conn.Close() //nolint:errcheck

				headersResp, bodyResp := helpers.SendHeadersAndBody(context.Background(), client, headersReq,
					toolCallBody("list_repositories"))
				if headersResp == nil || bodyResp == nil {
					return false
				}

				authMutated, err := helpers.HaveReplacedAuthorizationHeader("Bearer " + fixtures.FreshExchangedToken).Match(headersResp)
				if err != nil || !authMutated {
					return false
				}

				bodyForwarded, err := helpers.BeForwardedRequestBody().Match(bodyResp)
				return err == nil && bodyForwarded
			}, 10*time.Second, 200*time.Millisecond).Should(BeTrue(),
				"bundle-backed policy should eventually allow the request once OPA loads the local bundle")
		})

		// Scenario US4.2 from specs/020-extproc-opa-authorization/spec.md
		// Given an OPA config file with an unreachable bundle server,
		// When ExtProc starts,
		// Then startup follows OPA's own retry semantics.
		It("should follow OPA retry semantics when bundle server is unreachable", func() {
			// US4 Scenario 2 from specs/020-extproc-opa-authorization/spec.md
			//
			// OPA retry semantics: when the bundle server is initially unreachable,
			// OPA starts in non-blocking mode and retries until the bundle is loaded.
			// Requests are denied (fail-closed, IsUndefinedErr → deny) until the bundle loads.

			// bundleReadyCh gates the mock bundle server: returns HTTP 500 until closed.
			bundleReadyCh := make(chan struct{})
			// Inline allow-all policy embedded in the bundle (avoids filesystem dependency).
			allowAllPolicy := `package aib.extproc.authz

			import rego.v1

			allow contains {"action": "allow", "reason": "policy: all requests allowed"} if {
				true
			}

			result := decision if {
				count(allow) > 0
				decision := {"action": "allow"}
			} else := {"action": "deny", "reasons": ["default deny"]}`

			// Create a mock bundle server that returns HTTP 500 until bundleReadyCh is closed.
			bundleServer := sdktest.MustNewServer(
				sdktest.Ready(bundleReadyCh),
				sdktest.MockBundle("/bundles/bundle.tar.gz", map[string]string{
					"allow_all.rego": allowAllPolicy,
				}),
			)
			defer bundleServer.Stop()

			// Write an OPA config file pointing to the mock bundle server.
			tmpDir := GinkgoT().TempDir()
			opaConfigPath := filepath.Join(tmpDir, "opa-config.yaml")
			opaConfigContent := fmt.Sprintf(`services:
  bundle-server:
    url: %s
bundles:
  app:
    service: bundle-server
    resource: /bundles/bundle.tar.gz
    polling:
      min_delay_seconds: 1
      max_delay_seconds: 2
`, bundleServer.URL())
			Expect(os.WriteFile(opaConfigPath, []byte(opaConfigContent), 0600)).To(Succeed())

			// Build ExtProc config with OPA SDK bundle mode.
			cfg := fixtures.DefaultConfig()
			cfg.Authorization = extprocconfig.AuthorizationConfig{
				Enabled: true,
				Policy: extprocconfig.PolicyConfig{
					ConfigFile: opaConfigPath,
					Package:    "aib.extproc.authz",
					Decision:   "result",
				},
				DefaultDecision:   "deny",
				EvaluationTimeout: 2 * time.Second,
				MaxBodySize:       1024 * 1024,
			}

			// Use the production constructor and observe readiness through request behavior.
			authorizer, err := bootstrap.NewOPAAuthorizer(cfg, opaLogger)
			Expect(err).NotTo(HaveOccurred(), "OPA initialisation should not fail immediately")

			env := bootstrap.NewTestEnvironment(cfg, opaLogger)
			defer authorizer.Stop(context.Background()) //nolint:errcheck
			defer env.Stop()
			env.StartWithAuthorizer(authorizer)

			env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken)
			expiresIn := fixtures.StandardExpiresIn
			env.MockTokenExchange.WithExpiresIn(&expiresIn)

			client, conn := env.NewExtProcClient()
			defer conn.Close() //nolint:errcheck

			headersReq := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				WithHeader(":method", "POST").
				WithAgentgatewayProtocol("mcp").
				BuildWithMetadata()

			// Phase 1: bundle server unreachable — OPA returns IsUndefinedErr → fail-closed deny.
			_, bodyResp := helpers.SendHeadersAndBody(context.Background(), client, headersReq,
				toolCallBody("list_repositories"))
			Expect(bodyResp).NotTo(BeNil())
			Expect(bodyResp).To(helpers.HaveImmediateResponseWithStatus(403),
				"OPA should deny while bundle is unavailable (undefined result → fail-closed)")

			// Phase 2: make the bundle server available and observe the request path until OPA allows.
			close(bundleReadyCh)

			Eventually(func() bool {
				client, conn := env.NewExtProcClient()
				defer conn.Close() //nolint:errcheck

				headersResp, bodyResp := helpers.SendHeadersAndBody(context.Background(), client, headersReq,
					toolCallBody("list_repositories"))
				if headersResp == nil || bodyResp == nil {
					return false
				}

				authMutated, err := helpers.HaveReplacedAuthorizationHeader("Bearer " + fixtures.FreshExchangedToken).Match(headersResp)
				if err != nil || !authMutated {
					return false
				}

				bodyForwarded, err := helpers.BeForwardedRequestBody().Match(bodyResp)
				return err == nil && bodyForwarded
			}, 15*time.Second, 200*time.Millisecond).Should(BeTrue(),
				"OPA should eventually forward the request body after the bundle loads and allows the request")
		})
	})

	// -------------------------------------------------------------------------
	// User Story 5: Authorization is Optional (P5)
	// specs/020-extproc-opa-authorization/spec.md — US5
	// -------------------------------------------------------------------------
	Describe("US5: Authorization is Optional", func() {
		var (
			env        *bootstrap.TestEnvironment
			grpcClient extprocv3.ExternalProcessorClient
			conn       *grpc.ClientConn
		)

		BeforeEach(func() {
			cfg := opaDisabledConfig()
			env = bootstrap.NewTestEnvironment(cfg, opaLogger)
			env.Start()

			env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken)
			expiresIn := fixtures.StandardExpiresIn
			env.MockTokenExchange.WithExpiresIn(&expiresIn)

			grpcClient, conn = env.NewExtProcClient()
		})

		AfterEach(func() {
			conn.Close() //nolint:errcheck
			env.Stop()
		})

		// Scenario US5.1 from specs/020-extproc-opa-authorization/spec.md
		// Given no OPA configuration is provided,
		// When ExtProc starts,
		// Then the service processes requests using token exchange only.
		It("should process requests without OPA when not configured", func() {
			req := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				BuildWithMetadata()

			resp := helpers.SendRequestHeaders(context.Background(), grpcClient, req)

			Expect(resp).NotTo(BeNil())
			Expect(resp).To(helpers.HaveReplacedAuthorizationHeader("Bearer "+fixtures.FreshExchangedToken),
				"OPA disabled: request should be processed by token exchange only")

			// Token exchange should have been called (OPA did not intercept)
			Expect(resp.ModeOverride).To(BeNil(),
				"OPA disabled: headers response must not request body buffering")
			Expect(env.MockTokenExchange.CallCount()).To(Equal(1),
				"token exchange should be called when OPA is not configured")
		})

		// Scenario US5.2 from specs/020-extproc-opa-authorization/spec.md
		// Given OPA is explicitly disabled,
		// When a request with token-exchange metadata arrives,
		// Then no body inspection occurs and token exchange proceeds directly.
		It("should skip body inspection when OPA disabled", func() {
			headersReq := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				BuildWithMetadata()

			resp := helpers.SendRequestHeaders(context.Background(), grpcClient, headersReq)

			Expect(resp).NotTo(BeNil())
			Expect(resp.ModeOverride).To(BeNil(),
				"OPA disabled: headers phase should not request buffered body inspection")
			Expect(resp).To(helpers.HaveReplacedAuthorizationHeader("Bearer "+fixtures.FreshExchangedToken),
				"OPA disabled: headers phase alone should complete token exchange")
		})
	})

	// -------------------------------------------------------------------------
	// Edge Cases
	// specs/020-extproc-opa-authorization/spec.md — Edge Cases
	// -------------------------------------------------------------------------
	Describe("Edge Cases", func() {

		// Edge case: both policy sources specified
		// specs/020-extproc-opa-authorization/spec.md — Edge Cases
		It("should reject config with both local file and OPA config", func() {
			// Edge case from specs/020-extproc-opa-authorization/spec.md

			cfg := fixtures.DefaultConfig()
			cfg.Authorization = extprocconfig.AuthorizationConfig{
				Enabled: true,
				Policy: extprocconfig.PolicyConfig{
					Path:       policyPath("allow_all.rego"),
					ConfigFile: "/etc/extproc/opa-config.yaml", // Both set — invalid
					Package:    "aib.extproc.authz",
					Decision:   "result",
				},
				DefaultDecision:   "deny",
				EvaluationTimeout: 100 * time.Millisecond,
				MaxBodySize:       1024 * 1024,
			}

			err := extprocconfig.Validate(cfg)

			Expect(err).To(HaveOccurred(),
				"config with both path and config_file should fail validation")
			Expect(err.Error()).To(Or(
				ContainSubstring("path"),
				ContainSubstring("config_file"),
				ContainSubstring("mutually exclusive"),
				ContainSubstring("only one"),
			), "error should reference the mutual exclusivity constraint")
		})

		// Edge case: OPA evaluation timeout
		// specs/020-extproc-opa-authorization/spec.md — Edge Cases
		It("should deny on evaluation timeout", func() {
			// Edge case from specs/020-extproc-opa-authorization/spec.md

			// Use an extremely short timeout to force a timeout on any real policy evaluation
			cfg := fixtures.DefaultConfig()
			cfg.Authorization = extprocconfig.AuthorizationConfig{
				Enabled: true,
				Policy: extprocconfig.PolicyConfig{
					Path:     policyPath("allow_all.rego"),
					Package:  "aib.extproc.authz",
					Decision: "result",
				},
				DefaultDecision:   "deny",
				EvaluationTimeout: 1 * time.Nanosecond, // guaranteed timeout
				MaxBodySize:       1024 * 1024,
			}

			env := bootstrap.NewTestEnvironment(cfg, opaLogger)
			env.Start()
			defer env.Stop()

			client, conn := env.NewExtProcClient()
			defer conn.Close() //nolint:errcheck

			headersReq := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				WithHeader(":method", "POST").
				WithAgentgatewayProtocol("mcp").
				BuildWithMetadata()

			_, bodyResp := helpers.SendHeadersAndBody(context.Background(), client, headersReq,
				toolCallBody("list_repositories"))

			// On timeout, fail closed: return 403
			Expect(bodyResp).NotTo(BeNil())
			Expect(bodyResp).To(helpers.HaveImmediateResponseWithStatus(403),
				"OPA timeout should result in 403 deny (fail closed)")
		})
	})
})
