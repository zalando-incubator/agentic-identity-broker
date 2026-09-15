// Package helpers provides custom Gomega matchers for ExtProc ProcessingResponse assertions.
package helpers

import (
	"fmt"
	"strings"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

// HaveReplacedAuthorizationHeader asserts that the ProcessingResponse replaces
// the Authorization header with the expected Bearer token value.
// The expected value can be provided as a bare token (e.g. "mytoken") or as a
// full Authorization header value (e.g. "Bearer mytoken"); the "Bearer " prefix
// is added automatically when it is absent.
//
// Example:
//
//	Expect(resp).To(helpers.HaveReplacedAuthorizationHeader("exchanged-token-123"))
//	Expect(resp).To(helpers.HaveReplacedAuthorizationHeader("Bearer exchanged-token-123"))
func HaveReplacedAuthorizationHeader(expectedValue string) types.GomegaMatcher {
	if !strings.HasPrefix(expectedValue, "Bearer ") {
		expectedValue = "Bearer " + expectedValue
	}
	return &replacedAuthHeaderMatcher{expected: expectedValue}
}

type replacedAuthHeaderMatcher struct {
	expected string
	actual   string
}

func (m *replacedAuthHeaderMatcher) Match(actual interface{}) (success bool, err error) {
	resp, ok := actual.(*extprocv3.ProcessingResponse)
	if !ok {
		return false, fmt.Errorf("HaveReplacedAuthorizationHeader expects *extprocv3.ProcessingResponse, got %T", actual)
	}

	m.actual = ExtractMutatedAuthorizationHeader(resp)
	return m.actual == m.expected, nil
}

func (m *replacedAuthHeaderMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected ProcessingResponse to replace Authorization header with\n\t%q\nbut got\n\t%q",
		m.expected, m.actual)
}

func (m *replacedAuthHeaderMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected ProcessingResponse NOT to replace Authorization header with\n\t%q\nbut it did",
		m.expected)
}

// BeForwardedRequestBody asserts that the ProcessingResponse forwards the request body
// in the RequestBody phase. This is the allow-path response shape for body-bearing OPA
// requests after the headers-phase token exchange has already succeeded.
func BeForwardedRequestBody() types.GomegaMatcher {
	return &forwardedRequestBodyMatcher{}
}

type forwardedRequestBodyMatcher struct {
	actual string
}

func (m *forwardedRequestBodyMatcher) Match(actual interface{}) (success bool, err error) {
	resp, ok := actual.(*extprocv3.ProcessingResponse)
	if !ok {
		return false, fmt.Errorf("BeForwardedRequestBody expects *extprocv3.ProcessingResponse, got %T", actual)
	}

	if resp.Response == nil {
		m.actual = "<nil>"
		return false, nil
	}

	m.actual = fmt.Sprintf("%T", resp.Response)
	_, ok = resp.Response.(*extprocv3.ProcessingResponse_RequestBody)
	return ok, nil
}

func (m *forwardedRequestBodyMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected ProcessingResponse to forward the request body in the RequestBody phase but got %s", m.actual)
}

func (m *forwardedRequestBodyMatcher) NegatedFailureMessage(actual interface{}) string {
	return "Expected ProcessingResponse NOT to forward the request body in the RequestBody phase"
}

// HaveImmediateResponseWithStatus asserts that the ProcessingResponse is an
// ImmediateResponse with the specified HTTP status code.
//
// Example:
//
//	Expect(resp).To(helpers.HaveImmediateResponseWithStatus(503))
func HaveImmediateResponseWithStatus(expectedStatus uint32) types.GomegaMatcher {
	return &immediateResponseStatusMatcher{expected: expectedStatus}
}

type immediateResponseStatusMatcher struct {
	expected uint32
	actual   uint32
}

func (m *immediateResponseStatusMatcher) Match(actual interface{}) (success bool, err error) {
	resp, ok := actual.(*extprocv3.ProcessingResponse)
	if !ok {
		return false, fmt.Errorf("HaveImmediateResponseWithStatus expects *extprocv3.ProcessingResponse, got %T", actual)
	}

	m.actual = ExtractImmediateResponseStatus(resp)
	return m.actual == m.expected, nil
}

func (m *immediateResponseStatusMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected ProcessingResponse to be ImmediateResponse with status %d but got status %d",
		m.expected, m.actual)
}

func (m *immediateResponseStatusMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected ProcessingResponse NOT to be ImmediateResponse with status %d",
		m.expected)
}

// HaveImmediateResponseBody asserts that the ProcessingResponse's ImmediateResponse
// body contains the expected string.
//
// Example:
//
//	Expect(resp).To(helpers.HaveImmediateResponseBody(ContainSubstring("token_exchange_failed")))
func HaveImmediateResponseBody(bodyMatcher types.GomegaMatcher) types.GomegaMatcher {
	return &immediateResponseBodyMatcher{bodyMatcher: bodyMatcher}
}

type immediateResponseBodyMatcher struct {
	bodyMatcher types.GomegaMatcher
}

func (m *immediateResponseBodyMatcher) Match(actual interface{}) (success bool, err error) {
	resp, ok := actual.(*extprocv3.ProcessingResponse)
	if !ok {
		return false, fmt.Errorf("HaveImmediateResponseBody expects *extprocv3.ProcessingResponse, got %T", actual)
	}

	body := ExtractImmediateResponseBody(resp)
	return m.bodyMatcher.Match(body)
}

func (m *immediateResponseBodyMatcher) FailureMessage(actual interface{}) string {
	resp, ok := actual.(*extprocv3.ProcessingResponse)
	if !ok {
		return fmt.Sprintf("HaveImmediateResponseBody expects *extprocv3.ProcessingResponse, got %T", actual)
	}
	body := ExtractImmediateResponseBody(resp)
	return fmt.Sprintf("Expected ImmediateResponse body to match, but body was:\n\t%s\nMatcher failure: %s",
		body, m.bodyMatcher.FailureMessage(body))
}

func (m *immediateResponseBodyMatcher) NegatedFailureMessage(actual interface{}) string {
	return "Expected ImmediateResponse body NOT to match"
}

// HaveImmediateResponseWithBody asserts that the ProcessingResponse is an ImmediateResponse
// whose body contains the given substring. Convenience wrapper over HaveImmediateResponseBody.
//
// Example:
//
//	Expect(resp).To(helpers.HaveImmediateResponseWithBody("tool is destructive"))
func HaveImmediateResponseWithBody(containing string) types.GomegaMatcher {
	return HaveImmediateResponseBody(gomega.ContainSubstring(containing))
}
