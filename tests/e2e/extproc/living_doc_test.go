package extproc_test

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/livingdoc"
)

var _ = livingdoc.Record()

func example(id string) Labels { return livingdoc.Example(id) }
