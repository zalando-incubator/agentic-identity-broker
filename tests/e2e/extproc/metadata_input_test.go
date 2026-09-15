package extproc_test

import (
	"context"
	"net/http"
	"net/url"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
)

const (
	invalidSubjectTokenResponse = `{"error":"invalid_subject_token","error_description":"subject token metadata is missing or invalid"}`
	invalidResourceResponse     = `{"error":"invalid_resource","error_description":"resource metadata is missing or invalid"}`
)

func rawAttributeRequest() *helpers.ProcessingRequestBuilder {
	return helpers.NewRequestHeaders().
		WithPath(fixtures.AlternativeResourceURI).
		WithBearerToken(fixtures.AlternativeBearerToken)
}

func requestWithDirectMetadata(
	namespace string,
	fields map[string]*structpb.Value,
) *extprocv3.ProcessingRequest {
	req := rawAttributeRequest().Build()
	req.MetadataContext = &corev3.Metadata{
		FilterMetadata: map[string]*structpb.Struct{
			namespace: {Fields: fields},
		},
	}
	return req
}

func immediateResponseContentType(resp *extprocv3.ProcessingResponse) string {
	immediate, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	if !ok || immediate.ImmediateResponse == nil || immediate.ImmediateResponse.Headers == nil {
		return ""
	}
	for _, header := range immediate.ImmediateResponse.Headers.SetHeaders {
		if header.Header != nil && header.Header.Key == "content-type" {
			return string(header.Header.RawValue)
		}
	}
	return ""
}

func expectMetadataRejection(resp *extprocv3.ProcessingResponse, expectedBody string) {
	Expect(resp).NotTo(BeNil())
	Expect(resp).To(helpers.HaveImmediateResponseWithStatus(http.StatusServiceUnavailable))
	Expect(helpers.ExtractImmediateResponseBody(resp)).To(Equal(expectedBody))
	Expect(immediateResponseContentType(resp)).To(Equal("application/json"))
	Expect(helpers.ExtractMutatedAuthorizationHeader(resp)).To(BeEmpty())
}

func expectExchangeInputs(env *bootstrap.TestEnvironment, subjectToken, resourceURI string) {
	form, err := url.ParseQuery(env.MockTokenExchange.LastBody())
	Expect(err).NotTo(HaveOccurred())
	Expect(form.Get("subject_token")).To(Equal(subjectToken))
	Expect(form.Get("resource")).To(Equal(resourceURI))
}

var _ = Describe("ExtProc Metadata Input", func() {
	var (
		env    *bootstrap.TestEnvironment
		client extprocv3.ExternalProcessorClient
		conn   *grpc.ClientConn
		logger = bootstrap.NewTestLogger()
	)

	BeforeEach(func() {
		env = bootstrap.NewTestEnvironment(fixtures.DefaultConfig(), logger)
		env.Start()
		client, conn = env.NewExtProcClient()
	})

	AfterEach(func() {
		if conn != nil {
			conn.Close() //nolint:errcheck
		}
		if env != nil {
			env.Stop()
		}
	})

	Context("US1: Exchange From Dynamic Metadata", func() {
		// 043-extproc-metadata-input: US1-S2 — specs/043-extproc-metadata-input/spec.md
		It("uses only metadata values when raw HTTP attributes conflict", func() {
			env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken)
			req := rawAttributeRequest().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				BuildWithMetadata()

			resp := helpers.SendRequestHeaders(context.Background(), client, req)

			Expect(env.MockTokenExchange.CallCount()).To(Equal(1))
			expectExchangeInputs(env, fixtures.ValidBearerToken, fixtures.ValidResourceURI)
			Expect(resp).To(helpers.HaveReplacedAuthorizationHeader(fixtures.FreshExchangedToken))
		})

		// 043-extproc-metadata-input: US1-S3 — specs/043-extproc-metadata-input/spec.md
		It("rejects the request and forwards no credential when the exchange fails", func() {
			env.MockTokenExchange.WithError(http.StatusForbidden, "access_denied")
			req := helpers.NewRequestHeaders().
				WithoutAuthorizationHeader().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				BuildWithMetadata()

			resp := helpers.SendRequestHeaders(context.Background(), client, req)

			Expect(env.MockTokenExchange.CallCount()).To(Equal(1))
			Expect(resp).To(helpers.HaveImmediateResponseWithStatus(http.StatusInternalServerError))
			Expect(helpers.ExtractImmediateResponseBody(resp)).To(Equal(`{"error":"token_exchange_failed","error_description":"token exchange request failed"}`))
			Expect(immediateResponseContentType(resp)).To(Equal("application/json"))
			Expect(helpers.ExtractMutatedAuthorizationHeader(resp)).To(BeEmpty())
		})

		// 043-extproc-metadata-input: US1-S4 — specs/043-extproc-metadata-input/spec.md
		It("rejects without exchange when a required value is outside the namespace or is not a string", func() {
			testCases := []struct {
				name         string
				request      *extprocv3.ProcessingRequest
				expectedBody string
			}{
				{
					name: "outside the token-exchange namespace",
					request: requestWithDirectMetadata("aib.other", map[string]*structpb.Value{
						"subject_token": structpb.NewStringValue(fixtures.ValidBearerToken),
						"resource_uri":  structpb.NewStringValue(fixtures.ValidResourceURI),
					}),
					expectedBody: invalidSubjectTokenResponse,
				},
				{
					name: "subject token is not a string",
					request: requestWithDirectMetadata("aib.tokenexchange", map[string]*structpb.Value{
						"subject_token": structpb.NewNumberValue(1),
						"resource_uri":  structpb.NewStringValue(fixtures.ValidResourceURI),
					}),
					expectedBody: invalidSubjectTokenResponse,
				},
				{
					name: "resource URI is not a string",
					request: requestWithDirectMetadata("aib.tokenexchange", map[string]*structpb.Value{
						"subject_token": structpb.NewStringValue(fixtures.ValidBearerToken),
						"resource_uri":  structpb.NewBoolValue(true),
					}),
					expectedBody: invalidResourceResponse,
				},
			}

			for _, testCase := range testCases {
				By(testCase.name)
				resp := helpers.SendRequestHeaders(context.Background(), client, testCase.request)
				expectMetadataRejection(resp, testCase.expectedBody)
				Expect(env.MockTokenExchange.CallCount()).To(Equal(0))
			}
		})
	})

	Context("US2: Configure Instance Resource", func() {
		// 043-extproc-metadata-input: US2-S2 — specs/043-extproc-metadata-input/spec.md
		It("returns invalid_resource for missing, empty, non-string, or invalid resource metadata", func() {
			testCases := []struct {
				name    string
				request *extprocv3.ProcessingRequest
			}{
				{
					name: "missing resource URI",
					request: requestWithDirectMetadata("aib.tokenexchange", map[string]*structpb.Value{
						"subject_token": structpb.NewStringValue(fixtures.ValidBearerToken),
					}),
				},
				{
					name: "empty resource URI",
					request: rawAttributeRequest().
						WithTokenExchangeMetadata(fixtures.ValidBearerToken, "").
						BuildWithMetadata(),
				},
				{
					name: "non-string resource URI",
					request: requestWithDirectMetadata("aib.tokenexchange", map[string]*structpb.Value{
						"subject_token": structpb.NewStringValue(fixtures.ValidBearerToken),
						"resource_uri":  structpb.NewNumberValue(1),
					}),
				},
				{
					name: "relative resource URI",
					request: rawAttributeRequest().
						WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.RelativePathResourceURI).
						BuildWithMetadata(),
				},
			}

			for _, testCase := range testCases {
				By(testCase.name)
				resp := helpers.SendRequestHeaders(context.Background(), client, testCase.request)
				expectMetadataRejection(resp, invalidResourceResponse)
				Expect(env.MockTokenExchange.CallCount()).To(Equal(0))
			}
		})
	})

	Context("US3: Remove Raw Attribute Dependency", func() {
		// 043-extproc-metadata-input: US3-S1 — specs/043-extproc-metadata-input/spec.md
		It("returns invalid_subject_token for missing, empty, scheme-prefixed, or non-string subject metadata", func() {
			testCases := []struct {
				name    string
				request *extprocv3.ProcessingRequest
			}{
				{
					name: "missing subject token",
					request: requestWithDirectMetadata("aib.tokenexchange", map[string]*structpb.Value{
						"resource_uri": structpb.NewStringValue(fixtures.ValidResourceURI),
					}),
				},
				{
					name: "empty subject token",
					request: rawAttributeRequest().
						WithTokenExchangeMetadata("", fixtures.ValidResourceURI).
						BuildWithMetadata(),
				},
				{
					name: "scheme-prefixed subject token",
					request: rawAttributeRequest().
						WithTokenExchangeMetadata("Bearer "+fixtures.ValidBearerToken, fixtures.ValidResourceURI).
						BuildWithMetadata(),
				},
				{
					name: "non-string subject token",
					request: requestWithDirectMetadata("aib.tokenexchange", map[string]*structpb.Value{
						"subject_token": structpb.NewBoolValue(true),
						"resource_uri":  structpb.NewStringValue(fixtures.ValidResourceURI),
					}),
				},
			}

			for _, testCase := range testCases {
				By(testCase.name)
				resp := helpers.SendRequestHeaders(context.Background(), client, testCase.request)
				expectMetadataRejection(resp, invalidSubjectTokenResponse)
				Expect(env.MockTokenExchange.CallCount()).To(Equal(0))
			}
		})

		// 043-extproc-metadata-input: US3-S2 — specs/043-extproc-metadata-input/spec.md
		It("returns invalid_subject_token for a raw-attribute-only request", func() {
			resp := helpers.SendRequestHeaders(context.Background(), client, rawAttributeRequest().Build())

			expectMetadataRejection(resp, invalidSubjectTokenResponse)
			Expect(env.MockTokenExchange.CallCount()).To(Equal(0))
		})
	})

	Context("Edge Cases", func() {
		// 043-extproc-metadata-input: Edge case — no metadata, one value, or whitespace-only values; specs/043-extproc-metadata-input/spec.md
		It("returns the defined response for absent, partial, or whitespace-only metadata", func() {
			testCases := []struct {
				name         string
				request      *extprocv3.ProcessingRequest
				expectedBody string
			}{
				{
					name:         "no dynamic metadata",
					request:      helpers.NewRequestHeaders().Build(),
					expectedBody: invalidSubjectTokenResponse,
				},
				{
					name: "only subject token metadata",
					request: requestWithDirectMetadata("aib.tokenexchange", map[string]*structpb.Value{
						"subject_token": structpb.NewStringValue(fixtures.ValidBearerToken),
					}),
					expectedBody: invalidResourceResponse,
				},
				{
					name: "only resource URI metadata",
					request: requestWithDirectMetadata("aib.tokenexchange", map[string]*structpb.Value{
						"resource_uri": structpb.NewStringValue(fixtures.ValidResourceURI),
					}),
					expectedBody: invalidSubjectTokenResponse,
				},
				{
					name: "whitespace-only subject token",
					request: rawAttributeRequest().
						WithTokenExchangeMetadata(" \t\n", fixtures.ValidResourceURI).
						BuildWithMetadata(),
					expectedBody: invalidSubjectTokenResponse,
				},
				{
					name: "whitespace-only resource URI",
					request: rawAttributeRequest().
						WithTokenExchangeMetadata(fixtures.ValidBearerToken, " \t\n").
						BuildWithMetadata(),
					expectedBody: invalidResourceResponse,
				},
			}

			for _, testCase := range testCases {
				By(testCase.name)
				resp := helpers.SendRequestHeaders(context.Background(), client, testCase.request)
				expectMetadataRejection(resp, testCase.expectedBody)
				Expect(env.MockTokenExchange.CallCount()).To(Equal(0))
			}
		})

		// 043-extproc-metadata-input: Edge case — Bearer-prefixed subject token; specs/043-extproc-metadata-input/spec.md
		It("rejects a scheme-prefixed subject token without forwarding it", func() {
			req := rawAttributeRequest().
				WithTokenExchangeMetadata("Bearer "+fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				BuildWithMetadata()

			resp := helpers.SendRequestHeaders(context.Background(), client, req)

			expectMetadataRejection(resp, invalidSubjectTokenResponse)
			Expect(env.MockTokenExchange.CallCount()).To(Equal(0))
		})

		// 043-extproc-metadata-input: Edge case — resource URL is not absolute HTTP(S) with a host; specs/043-extproc-metadata-input/spec.md
		It("rejects resource values that are not absolute HTTP or HTTPS URLs with a host", func() {
			for _, resourceURI := range []string{
				fixtures.RelativePathResourceURI,
				fixtures.InvalidSchemeResourceURI,
				"https:///missing-host",
			} {
				resp := helpers.SendRequestHeaders(
					context.Background(),
					client,
					rawAttributeRequest().
						WithTokenExchangeMetadata(fixtures.ValidBearerToken, resourceURI).
						BuildWithMetadata(),
				)
				expectMetadataRejection(resp, invalidResourceResponse)
				Expect(env.MockTokenExchange.CallCount()).To(Equal(0))
			}
		})

		// 043-extproc-metadata-input: Edge case — both required metadata values are invalid; specs/043-extproc-metadata-input/spec.md
		It("returns invalid_subject_token when both required metadata values are invalid", func() {
			req := rawAttributeRequest().
				WithTokenExchangeMetadata("Bearer "+fixtures.ValidBearerToken, fixtures.RelativePathResourceURI).
				BuildWithMetadata()

			resp := helpers.SendRequestHeaders(context.Background(), client, req)

			expectMetadataRejection(resp, invalidSubjectTokenResponse)
			Expect(env.MockTokenExchange.CallCount()).To(Equal(0))
		})

		// 043-extproc-metadata-input: Edge case — unrelated fields in aib.tokenexchange; specs/043-extproc-metadata-input/spec.md
		It("ignores unrelated fields in the token-exchange metadata namespace", func() {
			env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken)
			req := helpers.NewRequestHeaders().
				WithoutAuthorizationHeader().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				BuildWithMetadata()
			req.MetadataContext.FilterMetadata["aib.tokenexchange"].Fields["unrelated"] = structpb.NewStringValue("ignored")

			resp := helpers.SendRequestHeaders(context.Background(), client, req)

			Expect(env.MockTokenExchange.CallCount()).To(Equal(1))
			expectExchangeInputs(env, fixtures.ValidBearerToken, fixtures.ValidResourceURI)
			Expect(resp).To(helpers.HaveReplacedAuthorizationHeader(fixtures.FreshExchangedToken))
		})
	})
})
