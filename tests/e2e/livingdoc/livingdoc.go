// Package livingdoc binds acceptance scenarios to living-documentation
// examples and records the evidence that the livingdoc checker compares
// with the pages under docs/specification/.
package livingdoc

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"

	"github.com/agentic-identity-broker/agentic-identity-broker/tools/livingdoc/evidence"
)

const (
	labelPrefix = "example:"
	redacted    = "[redacted]"
)

var credentialHeaders = []string{"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "X-Client-Assertion"}

// ExchangeSource starts reporting HTTP exchanges of a suite to record.
type ExchangeSource func(record func(evidence.Exchange))

// Example marks a scenario as the evidence for the example that pages cite as id.
func Example(id string) ginkgo.Labels {
	return ginkgo.Label(labelPrefix + id)
}

// Record registers evidence recording for the calling suite. Call it once at
// package level. It records the outcome of each scenario that carries an
// example, and the exchanges that the sources report while it runs.
func Record(sources ...ExchangeSource) bool {
	dir := os.Getenv(evidence.EvidenceDirEnv)
	if dir == "" {
		return false
	}
	var (
		mu        sync.Mutex
		recording bool
		exchanges []evidence.Exchange
	)
	for _, source := range sources {
		source(func(exchange evidence.Exchange) {
			mu.Lock()
			defer mu.Unlock()
			if recording {
				exchanges = append(exchanges, exchange)
			}
		})
	}
	ginkgo.ReportBeforeEach(func(report ginkgo.SpecReport) {
		mu.Lock()
		defer mu.Unlock()
		recording, exchanges = len(examples(report)) > 0, nil
	})
	ginkgo.ReportAfterEach(func(report ginkgo.SpecReport) {
		mu.Lock()
		recorded := exchanges
		recording, exchanges = false, nil
		mu.Unlock()
		for _, id := range examples(report) {
			record := evidence.Record{
				Example: id, Spec: report.FullText(), Location: location(report),
				State: state(report), Exchanges: recorded,
			}
			if err := evidence.Write(dir, record); err != nil {
				ginkgo.GinkgoWriter.Printf("living documentation evidence not written: %v\n", err)
			}
		}
	})
	return true
}

// HTTPExchange converts an observed request and response into evidence.
// Credential headers are redacted; bodies are kept as sent.
func HTTPExchange(request *http.Request, requestBody []byte, response *http.Response, responseBody []byte) evidence.Exchange {
	requestHeader, responseHeader := request.Header.Clone(), response.Header.Clone()
	for _, name := range credentialHeaders {
		if requestHeader.Get(name) != "" {
			requestHeader.Set(name, redacted)
		}
		if responseHeader.Get(name) != "" {
			responseHeader.Set(name, redacted)
		}
	}
	return evidence.Exchange{
		Method: request.Method, Target: request.URL.RequestURI(),
		RequestHeader: requestHeader, RequestBody: string(requestBody),
		Status: response.StatusCode, ResponseHeader: responseHeader, ResponseBody: string(responseBody),
	}
}

func location(report ginkgo.SpecReport) string {
	file := report.LeafNodeLocation.FileName
	if dir, err := os.Getwd(); err == nil {
		if relative, err := filepath.Rel(dir, file); err == nil {
			file = relative
		}
	}
	return fmt.Sprintf("%s:%d", filepath.ToSlash(file), report.LeafNodeLocation.LineNumber)
}

func examples(report ginkgo.SpecReport) []string {
	var ids []string
	for _, label := range report.Labels() {
		if id, ok := strings.CutPrefix(label, labelPrefix); ok {
			ids = append(ids, strings.TrimSpace(id))
		}
	}
	return ids
}

func state(report ginkgo.SpecReport) string {
	switch {
	case report.State.Is(types.SpecStatePassed):
		return evidence.StatePassed
	case report.State.Is(types.SpecStateSkipped | types.SpecStatePending):
		return evidence.StateSkipped
	default:
		return evidence.StateFailed
	}
}
