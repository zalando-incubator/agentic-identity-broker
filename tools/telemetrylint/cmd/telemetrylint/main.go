// Command telemetrylint runs the telemetrylint analyzer with its default settings outside
// golangci-lint, for example: go run ./cmd/telemetrylint ./... from the repository root module.
package main

import (
	"log"

	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/agentic-identity-broker/agentic-identity-broker/tools/telemetrylint"
)

func main() {
	analyzer, err := telemetrylint.NewAnalyzer(telemetrylint.Settings{})
	if err != nil {
		log.Fatal(err)
	}
	singlechecker.Main(analyzer)
}
