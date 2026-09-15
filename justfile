# Variable definitions
NAME := "agentic-identity-broker"
IMAGE_NAME := env_var_or_default("IMAGE_NAME", "agentic-identity-broker")
GINKGO_PROCS := env_var_or_default("GINKGO_PROCS", "4")
GINKGO_BACKEND_PROCS := env_var_or_default("GINKGO_BACKEND_PROCS", GINKGO_PROCS)
GINKGO_EXTPROC_PROCS := env_var_or_default("GINKGO_EXTPROC_PROCS", GINKGO_PROCS)
NUM_CPUS := num_cpus()
VERSION := `git describe --tags --always 2>/dev/null || echo "latest"`
GO_FAST_TEST_PACKAGES := `go list -e ./... | grep -Ev '(/assets/docusaurus/build/|/specs/|/web/node_modules/|/tests/e2e$|/tests/e2e/frontend$|/tests/e2e/extproc$|/tests/integration($|/))' | tr '\n' ' '`
INTEGRATION_INFRA_TEST_PACKAGES := "./tests/integration/infra/... ./tests/integration/migrations/... ./tests/integration/storage/infra/... ./internal/adapters/storage/postgres/..."
INTEGRATION_INFRA_PACKAGE_PROCS := env_var_or_default("INTEGRATION_INFRA_PACKAGE_PROCS", "2")
GINKGO_FRONTEND_PROCS := env_var_or_default("GINKGO_FRONTEND_PROCS", "2")
E2E_CAPTURE_SCREENSHOTS := env_var_or_default("E2E_CAPTURE_SCREENSHOTS", "false")
# JWX v4 requires jsonv2 only on Go 1.26; Go 1.27 includes it by default.
export GOEXPERIMENT := `case "$(go env GOVERSION)" in go1.26.*) printf 'jsonv2' ;; esac`

# Determine Docker Compose command (docker-compose or docker compose)
COMPOSE_CMD := `if [ -n "${COMPOSE_CMD:-}" ]; then echo "$COMPOSE_CMD"; elif command -v docker-compose >/dev/null 2>&1; then echo "docker-compose"; else echo "docker compose"; fi`
COMPOSE_FILE_ARGS := "--env-file .env.compose -f docker-compose.yml"


# Default recipe (shown when running `just` with no args)
default:
    @just --list

# =============================================================================
# Go Development Targets
# =============================================================================

# Build the Go binary for the host OS/ARCH
build:
    @echo "Building {{NAME}}..."
    @mkdir -p bin
    go build -ldflags="-s -w" -o bin/{{NAME}} ./cmd/{{NAME}}
    @echo "✓ Built: bin/{{NAME}}"

# Build Linux binary for arm64
build-linux-arm64:
    @echo "Building Linux binary for arm64..."
    @mkdir -p bin/linux/arm64
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/linux/arm64/{{NAME}} ./cmd/{{NAME}}
    @echo "✓ Built: bin/linux/arm64/{{NAME}}"

# Build Linux binary for amd64
build-linux-amd64:
    @echo "Building Linux binary for amd64..."
    @mkdir -p bin/linux/amd64
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/linux/amd64/{{NAME}} ./cmd/{{NAME}}
    @echo "✓ Built: bin/linux/amd64/{{NAME}}"

# Build macOS binary for arm64 (Apple Silicon)
build-darwin-arm64:
    @echo "Building macOS binary for arm64..."
    @mkdir -p bin/darwin/arm64
    GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/darwin/arm64/{{NAME}} ./cmd/{{NAME}}
    @echo "✓ Built: bin/darwin/arm64/{{NAME}}"

# Build Windows binary for amd64
build-windows-amd64:
    @echo "Building Windows binary for amd64..."
    @mkdir -p bin/windows/amd64
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/windows/amd64/{{NAME}}.exe ./cmd/{{NAME}}
    @echo "✓ Built: bin/windows/amd64/{{NAME}}.exe"

# Run the fast local Go/package test loop (no E2E or integration suites)
test:
    @echo "Running fast Go/package tests..."
    go test -v -race {{GO_FAST_TEST_PACKAGES}}

# Run fast Go/package tests and generate JUnit XML report for CI/CD
test-junit:
    @echo "Running fast Go/package tests with JUnit output..."
    @mkdir -p test-results
    @go test -json -race {{GO_FAST_TEST_PACKAGES}} > test-results/go-test-output.json; TEST_EXIT=$$?; go-junit-report -parser gojson < test-results/go-test-output.json > test-results/junit.xml; REPORT_EXIT=$$?; if [ $$REPORT_EXIT -ne 0 ]; then echo "go-junit-report failed (exit $$REPORT_EXIT)" >&2; exit $$REPORT_EXIT; fi; echo "JUnit report generated at test-results/junit.xml"; exit $$TEST_EXIT

# Run fast Go/package tests with coverage report
test-coverage:
    @echo "Running fast Go/package tests with coverage..."
    @mkdir -p coverage
    go test -v -race -coverprofile=coverage/coverage.out -covermode=atomic {{GO_FAST_TEST_PACKAGES}}
    go tool cover -html=coverage/coverage.out -o coverage/coverage.html
    @echo "Coverage report generated at coverage/coverage.html"

# Run fast Go/package tests and display coverage percentage
test-coverage-summary:
    @echo "Running fast Go/package tests with coverage summary..."
    @mkdir -p coverage
    go test -race -coverprofile=coverage/coverage.out -covermode=atomic {{GO_FAST_TEST_PACKAGES}}
    go tool cover -func=coverage/coverage.out

# Run the backend E2E acceptance suite with Ginkgo
test-e2e-backend: web-build
    @echo "Running backend E2E suite..."
    @if command -v ginkgo > /dev/null; then ginkgo -v --procs={{GINKGO_BACKEND_PROCS}} --label-filter="!performance" ./tests/e2e/; else echo "Error: ginkgo is not installed. Install it with: go install github.com/onsi/ginkgo/v2/ginkgo@latest"; exit 1; fi

# Run the SC-001 backend performance measurement separately from functional E2E tests
test-e2e-performance:
    @echo "Running backend E2E performance measurement..."
    @if command -v ginkgo > /dev/null; then ginkgo -v --procs=1 --label-filter="performance" ./tests/e2e/; else echo "Error: ginkgo is not installed. Install it with: go install github.com/onsi/ginkgo/v2/ginkgo@latest"; exit 1; fi

# Run the backend E2E acceptance suite with coverage report
test-e2e-backend-coverage: web-build
    @echo "Running backend E2E suite with coverage..."
    @mkdir -p coverage
    @if command -v ginkgo > /dev/null; then ginkgo -v --procs={{GINKGO_BACKEND_PROCS}} --label-filter="!performance" --cover --coverprofile=e2e-backend.out --output-dir=coverage ./tests/e2e/; go tool cover -html=coverage/e2e-backend.out -o coverage/e2e-backend.html; echo "Backend E2E coverage report generated at coverage/e2e-backend.html"; else echo "Error: ginkgo is not installed. Install it with: go install github.com/onsi/ginkgo/v2/ginkgo@latest"; exit 1; fi

# Watch the backend E2E acceptance suite during development
test-e2e-backend-watch: web-build
    @echo "Watching backend E2E suite..."
    @if command -v ginkgo > /dev/null; then ginkgo watch -v --label-filter="!performance" ./tests/e2e/; else echo "Error: ginkgo is not installed. Install it with: go install github.com/onsi/ginkgo/v2/ginkgo@latest"; exit 1; fi

# Run the ExtProc E2E acceptance suite with Ginkgo
test-e2e-extproc:
    @echo "Running ExtProc E2E suite..."
    @if command -v ginkgo > /dev/null; then \
        ginkgo -v --procs={{GINKGO_EXTPROC_PROCS}} ./tests/e2e/extproc/; \
    else \
        echo "Error: ginkgo is not installed. Install it with: go install github.com/onsi/ginkgo/v2/ginkgo@latest"; \
        exit 1; \
    fi

# Run the ExtProc E2E acceptance suite with coverage report
test-e2e-extproc-coverage:
    @echo "Running ExtProc E2E suite with coverage..."
    @mkdir -p coverage
    @if command -v ginkgo > /dev/null; then \
        ginkgo -v --procs={{GINKGO_EXTPROC_PROCS}} --cover --coverprofile=e2e-extproc.out --output-dir=coverage ./tests/e2e/extproc/; \
        go tool cover -html=coverage/e2e-extproc.out -o coverage/e2e-extproc.html; \
        echo "ExtProc E2E coverage report generated at coverage/e2e-extproc.html"; \
    else \
        echo "Error: ginkgo is not installed. Install it with: go install github.com/onsi/ginkgo/v2/ginkgo@latest"; \
        exit 1; \
    fi

# Run the frontend E2E acceptance suite against a built frontend bundle
test-e2e-frontend: web-build
    #!/usr/bin/env bash
    set -euo pipefail
    E2E_FRONTEND_MODE=built E2E_CAPTURE_SCREENSHOTS={{E2E_CAPTURE_SCREENSHOTS}} ginkgo -v --procs={{GINKGO_FRONTEND_PROCS}} --output-interceptor-mode=none ./tests/e2e/frontend/

# Run the frontend E2E acceptance suite against a Vite dev server
# NOTE: Requires 'just web-dev' running in another terminal
test-e2e-frontend-dev:
    #!/usr/bin/env bash
    set -euo pipefail
    E2E_FRONTEND_MODE=dev E2E_CAPTURE_SCREENSHOTS={{E2E_CAPTURE_SCREENSHOTS}} ginkgo -v --procs={{GINKGO_FRONTEND_PROCS}} --output-interceptor-mode=none ./tests/e2e/frontend/

# Run the frontend E2E acceptance suite with coverage report
test-e2e-frontend-coverage: web-build
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p coverage
    E2E_FRONTEND_MODE=built E2E_CAPTURE_SCREENSHOTS={{E2E_CAPTURE_SCREENSHOTS}} ginkgo -v --procs={{GINKGO_FRONTEND_PROCS}} --output-interceptor-mode=none --cover --coverprofile=e2e-frontend.out --output-dir=coverage ./tests/e2e/frontend/
    go tool cover -html=coverage/e2e-frontend.out -o coverage/e2e-frontend.html
    echo "Frontend E2E coverage report generated at coverage/e2e-frontend.html"

# Run all backend, ExtProc, and frontend E2E acceptance suites
test-e2e: test-e2e-backend test-e2e-extproc test-e2e-frontend
    @echo "All E2E suites completed"

# Run coverage for all backend, ExtProc, and frontend E2E acceptance suites
test-e2e-coverage: test-e2e-backend-coverage test-e2e-extproc-coverage test-e2e-frontend-coverage
    @echo "E2E coverage reports generated under coverage/"

# Watch all E2E acceptance suites during development
test-e2e-watch:
    @echo "Watching backend, ExtProc, and frontend E2E suites..."
    @if command -v ginkgo > /dev/null; then E2E_FRONTEND_MODE=built ginkgo watch -v --label-filter="!performance" ./tests/e2e/ ./tests/e2e/extproc/ ./tests/e2e/frontend/; else echo "Error: ginkgo is not installed. Install it with: go install github.com/onsi/ginkgo/v2/ginkgo@latest"; exit 1; fi


# Build and run the application
run: build
    @echo "Running {{NAME}}..."
    IDENTITY_BROKER_JWE_SIGNING_KEY=`./scripts/generate-jwe-key.sh` IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY=`./scripts/generate-jwe-key.sh` ./bin/{{NAME}}

# Run with Air for hot-reload development (requires air to be installed)
dev:
    IDENTITY_BROKER_JWE_SIGNING_KEY=`./scripts/generate-jwe-key.sh`
    @echo "Starting development server with hot reload..."
    @if command -v air > /dev/null; then \
        air; \
    else \
        echo "Error: air is not installed. Install it with: go install github.com/air-verse/air@latest"; \
        exit 1; \
    fi

# Clean build artifacts and temporary files
clean:
    @echo "Cleaning build artifacts..."
    rm -rf bin
    rm -rf build
    rm -rf coverage
    rm -rf test-results
    rm -rf web/node_modules web/dist
    @echo "Clean complete"

# Download and tidy Go module dependencies
deps:
    @echo "Tidying Go modules..."
    go mod tidy
    @echo "Downloading Go modules..."
    go mod download
    @echo "Verifying Go modules..."
    go mod verify

# Run golangci-lint if available
lint:
    @echo "Running linter..."
    @if command -v golangci-lint > /dev/null; then \
        golangci-lint run ./...; \
    else \
        echo "Warning: golangci-lint is not installed. Install it from: https://golangci-lint.run/usage/install/"; \
        echo "Running basic go vet instead..."; \
        go vet ./...; \
    fi

# Format Go code with gofmt
fmt:
    @echo "Formatting Go code..."
    gofmt -s -w .
    @echo "Format complete"

# Run go vet for static analysis
vet:
    @echo "Running go vet..."
    go vet ./...

# Install development tools (air, golangci-lint, go-junit-report)
install-tools:
    @echo "Installing development tools..."
    @command -v air          > /dev/null || go install github.com/air-verse/air@v1.63.6
    @command -v golangci-lint > /dev/null || bash scripts/golangci-lint-install.sh -b /usr/local/bin v2.11.4
    @golangci-lint --version 2>/dev/null | grep -q "version 2.11" || bash scripts/golangci-lint-install.sh -b /usr/local/bin v2.11.4
    @command -v go-junit-report > /dev/null || go install github.com/jstemmer/go-junit-report/v2@v2.1.0
    @go install github.com/onsi/ginkgo/v2/ginkgo@v2.32.1
    @if [ -d "$HOME/.cache/ms-playwright" ] && [ -n "$(ls -A "$HOME/.cache/ms-playwright" 2>/dev/null)" ] && [ -f "$HOME/.cache/ms-playwright-go/1.62.1/package/cli.js" ]; then \
        echo "Playwright driver and browsers already installed, skipping download"; \
    else \
        go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6201.1 install --with-deps; \
    fi
    @echo "Tools installation complete"

# Setup git hooks for quality checks
setup-hooks:
    @echo "Setting up git hooks..."
    git config core.hooksPath .githooks
    @echo "✓ Git hooks configured to use .githooks directory"
    @echo "Pre-commit hook will run: fmt, vet, lint"

# Run the default self-contained integration tests that do not require external infrastructure
test-integration:
    @echo "Running self-contained integration suites..."
    go test -v ./tests/integration/...

# Run infra-backed integration tests that require build tags and external infrastructure
test-integration-infra:
    @echo "Running infra-backed integration suites..."
    go test -tags=integration -p {{INTEGRATION_INFRA_PACKAGE_PROCS}} -v {{INTEGRATION_INFRA_TEST_PACKAGES}}

# Run both self-contained and infra-backed integration suites
test-integration-all: test-integration test-integration-infra
    @echo "All integration suites completed"

# Run all integration tests and generate JUnit XML reports
test-integration-junit:
    #!/usr/bin/env bash
    set +e

    echo "Running integration suites with JUnit output..."
    mkdir -p test-results

    go test -json ./tests/integration/... \
        > test-results/integration-self-contained-output.json 2>&1
    SELF_CONTAINED_EXIT=$?

    go test -json -tags=integration -p {{INTEGRATION_INFRA_PACKAGE_PROCS}} \
        {{INTEGRATION_INFRA_TEST_PACKAGES}} \
        > test-results/integration-infra-output.json 2>&1
    INFRA_EXIT=$?

    go-junit-report -parser gojson \
        < test-results/integration-self-contained-output.json > test-results/integration-self-contained-junit.xml || true
    go-junit-report -parser gojson \
        < test-results/integration-infra-output.json > test-results/integration-infra-junit.xml || true

    MERGER_EXIT=0
    if command -v npx > /dev/null; then
        npx -y junit-report-merger@9.0.3 \
            test-results/integration-junit.xml \
            test-results/integration-self-contained-junit.xml \
            test-results/integration-infra-junit.xml || MERGER_EXIT=$?
    else
        echo "⚠ npx not found, integration-junit.xml was not merged"
        MERGER_EXIT=1
    fi

    if [ $SELF_CONTAINED_EXIT -ne 0 ]; then
        echo ""
        echo "--- Self-contained integration output (FAILED) ---"
        cat test-results/integration-self-contained-output.json
    fi
    if [ $INFRA_EXIT -ne 0 ]; then
        echo ""
        echo "--- Infra-backed integration output (FAILED) ---"
        cat test-results/integration-infra-output.json
    fi

    if [ $SELF_CONTAINED_EXIT -ne 0 ] || [ $INFRA_EXIT -ne 0 ] || [ $MERGER_EXIT -ne 0 ]; then
        echo "✗ Integration suites failed"
        exit 1
    fi

    echo "✓ Integration JUnit report generated at test-results/integration-junit.xml"

# Run the full verification gate with JUnit reports for CI/CD
verify-junit:
    #!/usr/bin/env bash
    set +e

    JUNIT_REPORTS=(
        test-results/fast-junit.xml
        test-results/integration-self-contained-junit.xml
        test-results/integration-infra-junit.xml
        test-results/e2e-backend-junit.xml
        test-results/e2e-extproc-junit.xml
        test-results/e2e-frontend-junit.xml
        test-results/web-unit-junit.xml
        test-results/cdk-junit.xml
        test-results/mock-agent-junit.xml
        test-results/mock-oauth2-junit.xml
    )

    merge_junit_reports() {
        echo ""
        echo "==> Merging JUnit reports..."
        if command -v npx > /dev/null; then
            npx -y junit-report-merger@9.0.3 \
                test-results/all-tests-junit.xml \
                "${JUNIT_REPORTS[@]}"
        else
            echo "⚠ npx not found, junit-report-merger not available"
            return 1
        fi
    }

    finalize_verification() {
        VERIFICATION_EXIT=$?
        trap - EXIT
        merge_junit_reports
        MERGER_EXIT=$?

        if [ "$VERIFICATION_EXIT" -ne 0 ]; then
            if [ "$MERGER_EXIT" -eq 0 ]; then
                echo "✓ Merged JUnit report"
            else
                echo "✗ Merged JUnit report ($MERGER_EXIT)"
            fi
            exit "$VERIFICATION_EXIT"
        fi

        echo ""
        echo "=== Verification Summary ==="
        echo "✓ Fast/package suites"
        echo "✓ Integration suites"
        echo "✓ E2E suites"

        if [ "$MERGER_EXIT" -ne 0 ]; then
            echo "✗ Merged JUnit report ($MERGER_EXIT)"
            echo ""
            echo "✗ Verification completed but JUnit merge failed"
            exit "$MERGER_EXIT"
        fi

        echo "✓ Merged JUnit report"
        echo ""
        echo "✓ Verification JUnit report generated at test-results/all-tests-junit.xml"
        exit 0
    }

    echo "Running verification suite with JUnit output..."
    mkdir -p test-results coverage
    rm -f test-results/all-tests-junit.xml "${JUNIT_REPORTS[@]}"
    trap finalize_verification EXIT
    # ===== STAGE 1: FAST/PACKAGE SUITES =====
    echo ""
    echo "==> Stage 1: fast/package suites"

    go test -json -race -p {{NUM_CPUS}} {{GO_FAST_TEST_PACKAGES}} \
        > test-results/fast-tests-output.json 2>&1 &
    FAST_PID=$!
    echo "  [fast]       PID $FAST_PID"

    (cd web && if [ ! -d node_modules ]; then npm ci --silent; fi && npm test --silent -- --run --reporter=junit) \
        > test-results/web-unit-junit.xml 2>test-results/web-unit.log &
    WEB_UNIT_PID=$!
    echo "  [web-unit]   PID $WEB_UNIT_PID"

    (cd infra/cdk && go test -json -race ./...) \
        > test-results/cdk-tests-output.json 2>&1 &
    CDK_PID=$!
    echo "  [cdk]        PID $CDK_PID"

    (cd mocks/sample-agent && go test -json ./...) \
        > test-results/mock-sample-agent-output.json 2>&1 &
    MOCK_AGENT_PID=$!
    echo "  [mock-agent] PID $MOCK_AGENT_PID"

    (cd mocks/upstream-oauth2-server && go test -json ./internal/handlers/...) \
        > test-results/mock-oauth2-output.json 2>&1 &
    MOCK_OAUTH2_PID=$!
    echo "  [mock-oauth] PID $MOCK_OAUTH2_PID"

    wait $FAST_PID;        FAST_EXIT=$?
    wait $WEB_UNIT_PID;    WEB_UNIT_EXIT=$?
    wait $CDK_PID;         CDK_EXIT=$?
    wait $MOCK_AGENT_PID;  MOCK_AGENT_EXIT=$?
    wait $MOCK_OAUTH2_PID; MOCK_OAUTH2_EXIT=$?

    go-junit-report -parser gojson \
        < test-results/fast-tests-output.json > test-results/fast-junit.xml || true
    go-junit-report -parser gojson \
        < test-results/cdk-tests-output.json > test-results/cdk-junit.xml || true
    go-junit-report -parser gojson \
        < test-results/mock-sample-agent-output.json > test-results/mock-agent-junit.xml || true
    go-junit-report -parser gojson \
        < test-results/mock-oauth2-output.json > test-results/mock-oauth2-junit.xml || true

    if [ $FAST_EXIT -ne 0 ]; then
        echo ""
        echo "--- Fast Go/package test output (FAILED) ---"
        cat test-results/fast-tests-output.json
    fi
    if [ $WEB_UNIT_EXIT -ne 0 ]; then
        echo ""
        echo "--- Web unit test output (FAILED) ---"
        cat test-results/web-unit.log
        echo "--- Web unit JUnit report (FAILED) ---"
        cat test-results/web-unit-junit.xml
    fi
    if [ $CDK_EXIT -ne 0 ]; then
        echo ""
        echo "--- CDK test output (FAILED) ---"
        cat test-results/cdk-tests-output.json
    fi
    if [ $MOCK_AGENT_EXIT -ne 0 ]; then
        echo ""
        echo "--- Mock sample-agent test output (FAILED) ---"
        cat test-results/mock-sample-agent-output.json
    fi
    if [ $MOCK_OAUTH2_EXIT -ne 0 ]; then
        echo ""
        echo "--- Mock upstream OAuth2 test output (FAILED) ---"
        cat test-results/mock-oauth2-output.json
    fi

    if [ $FAST_EXIT -ne 0 ] || [ $WEB_UNIT_EXIT -ne 0 ] || [ $CDK_EXIT -ne 0 ] || \
       [ $MOCK_AGENT_EXIT -ne 0 ] || [ $MOCK_OAUTH2_EXIT -ne 0 ]; then
        echo "✗ Stage 1 failed"
        exit 1
    fi

    echo "✓ Stage 1 passed"

    # ===== STAGE 2: INTEGRATION SUITES =====
    echo ""
    echo "==> Stage 2: integration suites"

    echo "  [integration-self]   running"
    go test -json ./tests/integration/... \
        > test-results/integration-self-contained-output.json 2>&1
    INTEGRATION_SELF_CONTAINED_EXIT=$?

    echo "  [integration-infra]  running"
    go test -json -tags=integration -p {{INTEGRATION_INFRA_PACKAGE_PROCS}} \
        {{INTEGRATION_INFRA_TEST_PACKAGES}} \
        > test-results/integration-infra-output.json 2>&1
    INTEGRATION_INFRA_EXIT=$?

    go-junit-report -parser gojson \
        < test-results/integration-self-contained-output.json > test-results/integration-self-contained-junit.xml || true
    go-junit-report -parser gojson \
        < test-results/integration-infra-output.json > test-results/integration-infra-junit.xml || true

    if [ $INTEGRATION_SELF_CONTAINED_EXIT -ne 0 ]; then
        echo ""
        echo "--- Self-contained integration output (FAILED) ---"
        cat test-results/integration-self-contained-output.json
    fi
    if [ $INTEGRATION_INFRA_EXIT -ne 0 ]; then
        echo ""
        echo "--- Infra-backed integration output (FAILED) ---"
        cat test-results/integration-infra-output.json
    fi

    if [ $INTEGRATION_SELF_CONTAINED_EXIT -ne 0 ] || [ $INTEGRATION_INFRA_EXIT -ne 0 ]; then
        echo "✗ Stage 2 failed"
        exit 1
    fi

    echo "✓ Stage 2 passed"

    # ===== STAGE 3: E2E SUITES =====
    echo ""
    echo "==> Stage 3: E2E suites"

    (cd web && npm run build --silent) > test-results/web-build.log 2>&1
    WEB_BUILD_EXIT=$?
    if [ $WEB_BUILD_EXIT -ne 0 ]; then
        echo ""
        echo "--- Frontend build output (FAILED) ---"
        cat test-results/web-build.log
        echo "✗ Stage 3 failed"
        exit 1
    fi

    ginkgo run -v --procs={{GINKGO_BACKEND_PROCS}} --label-filter="!performance" \
        --junit-report=test-results/e2e-backend-junit.xml ./tests/e2e/ \
        > test-results/e2e-backend.log 2>&1 &
    E2E_BACKEND_PID=$!
    echo "  [e2e-backend]  PID $E2E_BACKEND_PID"

    ginkgo run -v --procs={{GINKGO_EXTPROC_PROCS}} \
        --junit-report=test-results/e2e-extproc-junit.xml ./tests/e2e/extproc/ \
        > test-results/e2e-extproc.log 2>&1 &
    E2E_EXTPROC_PID=$!
    echo "  [e2e-extproc]  PID $E2E_EXTPROC_PID"

    E2E_FRONTEND_MODE=built E2E_CAPTURE_SCREENSHOTS={{E2E_CAPTURE_SCREENSHOTS}} ginkgo run -v --procs={{GINKGO_FRONTEND_PROCS}} --output-interceptor-mode=none \
        --junit-report=test-results/e2e-frontend-junit.xml ./tests/e2e/frontend/ \
        > test-results/e2e-frontend.log 2>&1 &
    E2E_FRONTEND_PID=$!
    echo "  [e2e-frontend] PID $E2E_FRONTEND_PID"

    wait $E2E_BACKEND_PID;  E2E_BACKEND_EXIT=$?
    wait $E2E_EXTPROC_PID;  E2E_EXTPROC_EXIT=$?
    wait $E2E_FRONTEND_PID; E2E_FRONTEND_EXIT=$?

    if [ $E2E_BACKEND_EXIT -ne 0 ]; then
        echo ""
        echo "--- Backend E2E output (FAILED) ---"
        cat test-results/e2e-backend.log
    fi
    if [ $E2E_EXTPROC_EXIT -ne 0 ]; then
        echo ""
        echo "--- ExtProc E2E output (FAILED) ---"
        cat test-results/e2e-extproc.log
    fi
    if [ $E2E_FRONTEND_EXIT -ne 0 ]; then
        echo ""
        echo "--- Frontend E2E output (FAILED) ---"
        cat test-results/e2e-frontend.log
    fi

    if [ $E2E_BACKEND_EXIT -ne 0 ] || [ $E2E_EXTPROC_EXIT -ne 0 ] || [ $E2E_FRONTEND_EXIT -ne 0 ]; then
        echo "✗ Stage 3 failed"
        exit 1
    fi

    echo "✓ Stage 3 passed"


# Run the full local verification gate with E2E as the final guard layer
verify: check test web-test cdk-test mock-sample-agent-test mock-upstream-oauth2-test test-integration-all test-e2e
    @echo "Verification suite completed"

# Run static quality checks (format, vet, lint; no tests)
check: fmt vet lint
    @echo "Static quality checks passed!"

# =============================================================================
# Web Development Targets
# =============================================================================

# Install web dependencies
web-install:
    @echo "Installing web dependencies..."
    cd web && npm install

# Install web dependencies only when missing
web-ensure-deps:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -d web/node_modules ]; then
        echo "Web dependencies already installed"
    else
        echo "Installing web dependencies..."
        cd web && npm install
    fi

# Install web dependencies for CI with strict engine checking
web-ci:
    @echo "Installing web dependencies for CI..."
    npm config set engine-strict true
    cd web && npm ci

# Start web development server
web-dev:
    @echo "Starting web development server..."
    cd web && npm run dev

# Build web frontend
web-build: web-ensure-deps
    @echo "Building web frontend..."
    cd web && npm run build

# Run web frontend tests
web-test: web-ensure-deps
    @echo "Running web frontend tests..."
    cd web && npm test -- --run

# Run web frontend tests with coverage
web-test-coverage: web-ensure-deps
    @echo "Running web frontend tests with coverage..."
    cd web && npm run test:coverage

# Build both Go backend and web frontend in release quality
# Produces artifacts: ./bin/{{NAME}} and ./web/dist/
build-all: build web-build
    @echo "✓ Build complete: Go backend and web frontend"

# =============================================================================
# Docker Targets
# =============================================================================

# Create and push multi-architecture Docker images to registry
# Builds broker, migrate, and extproc images for linux/amd64 and linux/arm64 using Docker Buildx
# Optional: set BUILDKIT_CONFIG to a buildx config file path (defaults to /etc/cdp-buildkitd.toml if present)
docker-push: build-linux-amd64 build-linux-arm64 extproc-build-linux-amd64 extproc-build-linux-arm64 web-build
    @echo "Building and pushing multi-architecture images..."
    @echo "Building broker image: {{IMAGE_NAME}}:{{VERSION}}..."
    docker_buildx build --rm -t "{{IMAGE_NAME}}:{{VERSION}}" --build-arg VERSION="{{VERSION}}" --platform linux/amd64,linux/arm64 --push .
    @echo "Building migrate image: {{IMAGE_NAME}}-migrate:{{VERSION}}..."
    docker_buildx build --rm -t "{{IMAGE_NAME}}-migrate:{{VERSION}}" --build-arg VERSION="{{VERSION}}" --platform linux/amd64,linux/arm64 --file Dockerfile.migrate --push .
    @echo "Building extproc image: {{IMAGE_NAME}}-extproc:{{VERSION}}..."
    docker_buildx build --rm -t "{{IMAGE_NAME}}-extproc:{{VERSION}}" --build-arg VERSION="{{VERSION}}" --platform linux/amd64,linux/arm64 --file Dockerfile.extproc --push .
    @echo "✓ Multi-architecture images pushed:"
    @echo "  - {{IMAGE_NAME}}:{{VERSION}}"
    @echo "  - {{IMAGE_NAME}}-migrate:{{VERSION}}"
    @echo "  - {{IMAGE_NAME}}-extproc:{{VERSION}}"

#TODO: Remove for OSS
docker-promote:
    @echo "Promoting docker images to production channel..."
    cdp-promote-image {{IMAGE_NAME}}:{{VERSION}}
    cdp-promote-image {{IMAGE_NAME}}-migrate:{{VERSION}}
    cdp-promote-image {{IMAGE_NAME}}-extproc:{{VERSION}}

# Build multi-architecture migrate Docker image (validates both platforms, no output)
docker-build-migrate:
    @echo "Building migrate image: {{IMAGE_NAME}}-migrate:{{VERSION}}..."
    docker_buildx build --rm -t "{{IMAGE_NAME}}-migrate:{{VERSION}}" --build-arg VERSION="{{VERSION}}" --platform linux/amd64,linux/arm64 --file Dockerfile.migrate .
    @echo "✓ Migrate image validated: {{IMAGE_NAME}}-migrate:{{VERSION}}"

# Build multi-architecture broker Docker image (validates both platforms, no output)
docker-build-broker: build-linux-amd64 build-linux-arm64 web-build
    @echo "Building broker image: {{IMAGE_NAME}}:{{VERSION}}..."
    docker_buildx build --rm -t "{{IMAGE_NAME}}:{{VERSION}}" --build-arg VERSION="{{VERSION}}" --platform linux/amd64,linux/arm64 .
    @echo "✓ Broker image validated: {{IMAGE_NAME}}:{{VERSION}}"

# Build multi-architecture extproc Docker image and smoke-test its native release image
docker-build-extproc: extproc-build-linux-amd64 extproc-build-linux-arm64
    @echo "Building extproc image: {{IMAGE_NAME}}-extproc:{{VERSION}}..."
    docker_buildx build --rm -t "{{IMAGE_NAME}}-extproc:{{VERSION}}" --build-arg VERSION="{{VERSION}}" --platform linux/amd64,linux/arm64 --file Dockerfile.extproc .
    docker_buildx build --rm --load -t "{{IMAGE_NAME}}-extproc:{{VERSION}}-smoke" --build-arg VERSION="{{VERSION}}" --file Dockerfile.extproc .
    docker run --rm "{{IMAGE_NAME}}-extproc:{{VERSION}}-smoke" ./extproc-token-exchange --help
    docker image rm "{{IMAGE_NAME}}-extproc:{{VERSION}}-smoke" > /dev/null
    @echo "✓ ExtProc image validated: {{IMAGE_NAME}}-extproc:{{VERSION}}"

# Build broker and migrate multi-architecture images and smoke-test ExtProc's native release image
docker-build-all: docker-build-broker docker-build-migrate docker-build-extproc
    @echo "✓ All Docker images validated for linux/amd64,linux/arm64; ExtProc native release image smoke-tested"

# =============================================================================
# Docker Compose - Development (Hot Reload)
# =============================================================================

# Create .env.compose from .env template if it doesn't exist
compose-env:
    @if [ -f .env.compose ]; then \
        echo ".env.compose already exists"; \
    else \
        cp .env .env.compose; \
        echo "✓ Created: .env.compose (customize as needed)"; \
    fi

# Validate docker-compose.yml syntax
compose-validate:
    @echo "Validating docker-compose.yml..."
    @{{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} config > /dev/null && echo "✓ Syntax valid" || echo "✗ Syntax error"

# Start all services with logs streaming (foreground)
compose-up: compose-env
    @echo "Generating JWE signing key..."
    @echo "Generating encryption key..."
    @echo "Starting docker-compose services with hot reload..."
    @echo "Services:"
    @echo "  - Identity Broker (8000, 14000) with Air hot reload"
    @echo "  - Frontend (3000) with Vite HMR"
    @echo "  - Upstream OAuth2 (9001)"
    @echo "  - Third-Party OAuth2 (9000)"
    @echo "  - Sample Agent (9002)"
    @echo "  - Seed data will auto-run once broker is healthy"
    @echo ""
    @echo "Press Ctrl+C to stop"
    IDENTITY_BROKER_JWE_SIGNING_KEY=`./scripts/generate-jwe-key.sh` IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY=`./scripts/generate-jwe-key.sh` {{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} up --build

# Start all services in background
compose-up-detached: compose-env
    @echo "Generating JWE signing key..."
    @echo "Starting docker-compose services in background..."
    @IDENTITY_BROKER_JWE_SIGNING_KEY=`./scripts/generate-jwe-key.sh` IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY=`./scripts/generate-jwe-key.sh` {{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} up -d --build
    @sleep 2
    @just compose-health
    @echo ""
    @echo "Service URLs:"
    @echo "  - Browser UI (Vite proxy includes development identity): http://localhost:3000/"
    @echo "  - End-user broker (protected API requires upstream auth): http://localhost:8000/"
    @echo "  - Admin API: http://localhost:14000/api/agents"
    @echo "  - Admin health: http://localhost:14000/health"
    @echo "  - Sample OAuth2 client: http://localhost:9002/oauth2/authorize"
    @echo ""
    @echo "View logs: just compose-logs"
    @echo "Stop services: just compose-down"

# Stop all services
compose-down:
    @echo "Stopping docker-compose services..."
    {{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} down

# Stop all services and remove volumes
compose-down-volumes:
    @echo "Stopping docker-compose services and removing volumes..."
    {{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} down -v

# View logs from all services (tail -f)
compose-logs:
    {{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} logs -f

# View logs from backend only
compose-logs-backend:
    {{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} logs -f identity-broker

# View logs from frontend only
compose-logs-frontend:
    {{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} logs -f frontend

# View logs from specific service
compose-logs-service SERVICE:
    {{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} logs -f {{SERVICE}}

# Restart backend service (after code changes)
compose-restart-backend:
    @echo "Restarting identity-broker service..."
    {{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} restart identity-broker

# Restart frontend service (after code changes)
compose-restart-frontend:
    @echo "Restarting frontend service..."
    {{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} restart frontend

# Show service status and connectivity
compose-health:
    @echo "Checking service health..."
    @{{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} ps
    @echo ""
    @echo "Testing connectivity..."
    @{{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} exec -T identity-broker curl -s http://localhost:8000/health && echo "✓ Backend health OK" || echo "✗ Backend not ready"
    @{{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} exec -T frontend curl -s http://localhost:3000 > /dev/null && echo "✓ Frontend responding" || echo "✗ Frontend not ready"
    @echo ""

# Clean up: stop containers, remove volumes, clean tmp directories
compose-clean: compose-down-volumes
    @echo "Tearing down worktree compose projects..."
    @find .worktrees -maxdepth 2 -name docker-compose.yml | while read -r f; do \
        {{COMPOSE_CMD}} -f "$$f" down -v 2>/dev/null || true; \
    done
    @echo "Cleaning up build and temporary directories..."
    @rm -rf tmp/ coverage/ bin/ web/dist web/node_modules
    @echo "✓ Cleanup complete"

# =============================================================================
# Helm Chart Targets
# =============================================================================

# Lint Helm chart for syntax and best practices
helm-lint:
    @echo "Linting Helm chart..."
    @helm lint charts/agentic-identity-broker
    @echo "✓ Helm chart lint passed"

# Validate Helm chart deployment in Kind cluster (full E2E test)
helm-validate:
    @echo "Validating Helm chart in Kind cluster..."
    @./scripts/validate-helm-chart.sh

# Render Helm templates (dry-run)
helm-template:
    @echo "Rendering Helm templates..."
    @helm template broker ./charts/agentic-identity-broker

# Render Helm templates with custom values
helm-template-values VALUES_FILE:
    @echo "Rendering Helm templates with {{VALUES_FILE}}..."
    @helm template broker ./charts/agentic-identity-broker -f {{VALUES_FILE}}

# Install Helm chart to local Kind cluster
helm-install-kind RELEASE_NAME="broker":
    @echo "Installing Helm chart to Kind cluster..."
    @kind create cluster --name helm-test 2>/dev/null || echo "Kind cluster already exists"
    @helm install {{RELEASE_NAME}} ./charts/agentic-identity-broker --wait
    @echo "✓ Chart installed as {{RELEASE_NAME}}"
    @echo ""
    @echo "Check status: kubectl get pods"
    @echo "Uninstall: helm uninstall {{RELEASE_NAME}}"

# Uninstall Helm chart from Kind cluster
helm-uninstall-kind RELEASE_NAME="broker":
    @echo "Uninstalling Helm chart..."
    @helm uninstall {{RELEASE_NAME}}
    @echo "✓ Chart uninstalled"

# Delete Kind test cluster
helm-kind-delete:
    @echo "Deleting Kind test cluster..."
    @kind delete cluster --name helm-test
    @echo "✓ Kind cluster deleted"

# Package Helm chart for distribution
helm-package:
    @echo "Packaging Helm chart..."
    @mkdir -p dist
    @helm package charts/agentic-identity-broker -d dist
    @echo "✓ Chart packaged to dist/"

# =============================================================================
# Documentation Targets
# =============================================================================

# Install documentation dependencies
docs-install:
    cd assets/docusaurus && npm install

# Build documentation site
docs-build: docs-install
    cd assets/docusaurus && npm run build

# Serve documentation locally (hot reload)
docs-serve:
    cd assets/docusaurus && npm start

# Serve production build locally for testing
docs-preview: docs-build
    cd assets/docusaurus && npm run serve

# Clean build artifacts
docs-clean:
    cd assets/docusaurus && npm run clear
    rm -rf assets/docusaurus/build assets/docusaurus/.docusaurus

# Run markdown linting
docs-lint:
    npx markdownlint-cli2 "docs/**/*.md"

# Check for broken links
docs-check-links: docs-build
    npx broken-link-checker http://localhost:3000

# Combined: install, build, preview
docs: docs-install docs-build docs-preview

# Deploy to GitHub Pages
docs-deploy: docs-build
    @test -n "${DOCS_SITE_URL:-}" || (echo "Set DOCS_SITE_URL to https://<custom-domain> before deploying" >&2; exit 1)
    @test -n "${DOCS_GITHUB_ORG:-}" || (echo "Set DOCS_GITHUB_ORG before deploying" >&2; exit 1)
    @test -n "${DOCS_GITHUB_REPO:-}" || (echo "Set DOCS_GITHUB_REPO before deploying" >&2; exit 1)
    @printf '%s\n' "$DOCS_SITE_URL" | sed 's#^https\{0,1\}://##; s#/.*$##' > assets/docusaurus/build/CNAME
    cd assets/docusaurus && npm run deploy -- --skip-build

# =============================================================================
# ExtProc Token Exchange Service Targets
# =============================================================================

# Build ExtProc Linux binary for arm64
extproc-build-linux-arm64:
    @echo "Building ExtProc Linux binary for arm64..."
    @mkdir -p bin/linux/arm64
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/linux/arm64/extproc-token-exchange ./cmd/extproc-token-exchange
    @echo "✓ Built: bin/linux/arm64/extproc-token-exchange"

# Build ExtProc Linux binary for amd64
extproc-build-linux-amd64:
    @echo "Building ExtProc Linux binary for amd64..."
    @mkdir -p bin/linux/amd64
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/linux/amd64/extproc-token-exchange ./cmd/extproc-token-exchange
    @echo "✓ Built: bin/linux/amd64/extproc-token-exchange"

# Build the extproc-token-exchange binary
extproc-build:
    @echo "Building extproc-token-exchange..."
    @mkdir -p bin
    go build -ldflags="-s -w" -o bin/extproc-token-exchange ./cmd/extproc-token-exchange
    @echo "✓ Built: bin/extproc-token-exchange"

# Run the extproc-token-exchange binary (requires EXTPROC_* env vars)
extproc-run: extproc-build
    @echo "Running extproc-token-exchange..."
    @echo "Set EXTPROC_CONFIG_PATH or EXTPROC_OAUTH2_TOKEN_ENDPOINT etc. before running."
    ./bin/extproc-token-exchange

# Run unit tests for the extproc packages
extproc-test:
    @echo "Running extproc unit tests..."
    go test -v -race ./internal/extproc/... ./cmd/extproc-token-exchange/...


# Build mock MCP server binary
mock-mcp-server-build:
    @echo "Building mock MCP server..."
    @mkdir -p bin
    cd mocks/mcp-server && go build -o ../../bin/mock-mcp-server cmd/mcp-server/main.go
    @echo "✓ Built: ./bin/mock-mcp-server"

# Run mock MCP server locally
mock-mcp-server-start:
    @echo "Starting mock MCP server on port 9003..."
    cd mocks/mcp-server && go run cmd/mcp-server/main.go

# Start the full ExtProc + agentgateway + MCP integration stack
compose-extproc-up:
    @echo "Starting ExtProc integration stack..."
    @echo "Services:"
    @echo "  - extproc-token-exchange (50051, container-internal)"
    @echo "  - mcp-server-mock (9003)"
    @echo "  - agentgateway (4000, 15000)"
    @echo ""
    @echo "Requires identity-broker and upstream-oauth2 to be running."
    @IDENTITY_BROKER_JWE_SIGNING_KEY=`./scripts/generate-jwe-key.sh` IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY=`./scripts/generate-jwe-key.sh` {{COMPOSE_CMD}} -f docker-compose.yml up extproc-token-exchange mcp-server-mock agentgateway

# Stop ExtProc integration services only
compose-extproc-down:
    @echo "Stopping ExtProc integration services..."
    {{COMPOSE_CMD}} -f docker-compose.yml stop extproc-token-exchange mcp-server-mock agentgateway

# Show logs from ExtProc integration services
compose-extproc-logs:
    {{COMPOSE_CMD}} -f docker-compose.yml logs -f extproc-token-exchange mcp-server-mock agentgateway

# =============================================================================
# Mock Third-Party OAuth2 Service Targets (Manual Testing)
# =============================================================================

# Start mock third-party OAuth2 service (port 9000)
mock-third-party-oauth2-start:
    @echo "Starting mock third-party OAuth2 service..."
    cd mocks/third-party-service && go run cmd/mock-oauth2-server/main.go

# Build mock third-party OAuth2 service binary
mock-third-party-oauth2-build:
    @echo "Building mock third-party OAuth2 service..."
    @mkdir -p bin
    cd mocks/third-party-service && go build -o ../../bin/mock-oauth2-server cmd/mock-oauth2-server/main.go
    @echo "✓ Binary built: ./bin/mock-oauth2-server"

# Register mock third-party service with broker admin API
mock-third-party-oauth2-register:
    @echo "Registering mock third-party OAuth2 service with broker..."
    @bash scripts/register-mock-thirdparty-service.sh

# Full setup: build, start (background), register
mock-third-party-oauth2-setup: mock-third-party-oauth2-build
    @echo "Setting up mock third-party OAuth2 testing environment..."
    @echo "1. Starting mock third-party OAuth2 server (background)..."
    @./bin/mock-oauth2-server mocks/third-party-service &
    @sleep 2
    @echo "2. Checking mock server health..."
    @curl -s -f http://localhost:9000/health || (echo "Mock server failed to start"; exit 1)
    @echo "   ✓ Mock server healthy"
    @echo "3. Registering mock service with broker..."
    @just mock-third-party-oauth2-register
    @echo ""
    @echo "========================================="
    @echo "Mock Third-Party OAuth2 Setup Complete!"
    @echo "========================================="
    @echo "Mock OAuth2 Server: http://localhost:9000"
    @echo "Broker Consent UI: http://localhost:8000/sessions"
    @echo ""
    @echo "To stop: pkill -f mock-oauth2-server"

# Stop and clean mock third-party OAuth2 artifacts
mock-third-party-oauth2-clean:
    @echo "Stopping mock third-party OAuth2 server..."
    @pkill -f mock-oauth2-server || true
    @rm -f bin/mock-oauth2-server
    @echo "✓ Mock cleanup complete"

# =============================================================================
# Mock Upstream OAuth2 Server Targets (Manual Testing)
# =============================================================================

# Start mock upstream OAuth2 server (port 9001) - visually distinctive
mock-upstream-oauth2-start:
    @echo "Starting mock upstream OAuth2 server..."
    cd mocks/upstream-oauth2-server && go run cmd/mock-upstream-oauth2-server/main.go

# Build mock upstream OAuth2 server binary
mock-upstream-oauth2-build:
    @echo "Building mock upstream OAuth2 server..."
    @mkdir -p bin
    cd mocks/upstream-oauth2-server && go build -o ../../bin/mock-upstream-oauth2-server cmd/mock-upstream-oauth2-server/main.go
    @echo "✓ Binary built: ./bin/mock-upstream-oauth2-server"

# Run tests for upstream OAuth2 mock server
mock-upstream-oauth2-test:
    @echo "Running tests for upstream OAuth2 mock server..."
    cd mocks/upstream-oauth2-server && go test -v ./internal/handlers/...

# Full setup: build, start (background), health check
mock-upstream-oauth2-setup: mock-upstream-oauth2-build
    @echo "Setting up mock upstream OAuth2 testing environment..."
    @echo "1. Starting mock upstream OAuth2 server (background)..."
    @./bin/mock-upstream-oauth2-server &
    @sleep 2
    @echo "2. Checking mock server health..."
    @curl -s -f http://localhost:9001/health || (echo "Mock server failed to start"; exit 1)
    @echo "   ✓ Mock server healthy"
    @echo ""
    @echo "========================================="
    @echo "Mock Upstream OAuth2 Setup Complete!"
    @echo "========================================="
    @echo "Mock Upstream OAuth2 Server: http://localhost:9001"
    @echo "Consent Page: http://localhost:9001/oauth/authorize?client_id=upstream-oauth2-client&response_type=code&redirect_uri=http://localhost/callback&scope=openid&state=test123"
    @echo ""
    @echo "Note: You'll see DISTINCTIVE TEAL background (#00d4aa)"
    @echo "      with 🌐 UPSTREAM OAUTH2 badge"
    @echo ""
    @echo "To stop: pkill -f mock-upstream-oauth2-server"

# Stop and clean mock upstream OAuth2 artifacts
mock-upstream-oauth2-clean:
    @echo "Stopping mock upstream OAuth2 server..."
    @pkill -f mock-upstream-oauth2-server || true
    @rm -f bin/mock-upstream-oauth2-server
    @echo "✓ Mock cleanup complete"

# =============================================================================
# Mock Sample OAuth2 Client Agent Targets (End-to-End Testing)
# =============================================================================

# Start mock sample OAuth2 client agent (port 8001)
mock-sample-agent-start:
    @echo "Starting mock sample OAuth2 client agent..."
    cd mocks/sample-agent && go run cmd/sample-agent/main.go

# Build mock sample OAuth2 client agent binary
mock-sample-agent-build:
    @echo "Building mock sample OAuth2 client agent..."
    @mkdir -p bin
    cd mocks/sample-agent && go build -o ../../bin/sample-agent cmd/sample-agent/main.go
    @echo "✓ Binary built: ./bin/sample-agent"

# Run tests for sample agent
mock-sample-agent-test:
    @echo "Running tests for sample agent..."
    cd mocks/sample-agent && go test -v ./...

# Full setup: build, start (background), health check
mock-sample-agent-setup: mock-sample-agent-build
    @echo "Setting up mock sample OAuth2 client testing environment..."
    @echo "1. Starting mock sample OAuth2 client (background)..."
    @./bin/sample-agent &
    @sleep 2
    @echo "2. Checking mock client health..."
    @curl -s -f http://localhost:8001/health || (echo "Sample client failed to start"; exit 1)
    @echo "   ✓ Sample client healthy"
    @echo ""
    @echo "========================================="
    @echo "Sample OAuth2 Client Setup Complete!"
    @echo "========================================="
    @echo ""
    @echo "Three-Tier OAuth2 Setup:"
    @echo "  Tier 1: Sample Agent ................ http://localhost:8001"
    @echo "  Tier 2: Identity Broker ............ http://localhost:8000"
    @echo "  Tier 3: Upstream OAuth2 Server .... http://localhost:9001"
    @echo ""
    @echo "Testing the complete end-to-end flow:"
    @echo "  1. Start this: just mock-sample-agent-setup"
    @echo "  2. Start broker: just dev"
    @echo "  3. Start upstream: just mock-upstream-oauth2-start"
    @echo "  4. Browser: http://localhost:8001"
    @echo "  5. Click Login with OAuth2"
    @echo "  6. Approve consent (notice TEAL background on port 9001)"
    @echo "  7. See user information page"
    @echo ""
    @echo "To stop: pkill -f 'sample-agent|bin/sample-agent'"

# Stop and clean mock sample agent artifacts
mock-sample-agent-clean:
    @echo "Stopping mock sample OAuth2 client agent..."
    @pkill -f 'sample-agent|bin/sample-agent' || true
    @rm -f bin/sample-agent
    @echo "✓ Mock cleanup complete"

# =============================================================================
# OPA Policy Targets
# =============================================================================

# Validate and build the OPA policy bundle (requires Docker)
opa-check:
    @echo "Checking OPA policy bundle..."
    ./mocks/opa/test-bundle.sh
    @echo "✓ OPA policy bundle OK"

# Rebuild the OPA bundle in the running compose stack; nginx picks it up automatically
opa-reload:
    {{COMPOSE_CMD}} {{COMPOSE_FILE_ARGS}} run --rm opa-bundle-build

# =============================================================================
# CDK Infrastructure Targets
# =============================================================================

# Install CDK dependencies (run once after cloning)
cdk-deps:
    @echo "Installing CDK Go dependencies..."
    cd infra/cdk && go mod tidy && go mod download
    @echo "✓ CDK dependencies installed"

# Run CDK unit tests
cdk-test:
    @echo "Running CDK stack tests..."
    cd infra/cdk && go test -v -race ./...
    @echo "✓ CDK tests passed"

# Synthesize CloudFormation template (default: test)
cdk-synth env="test" *ARGS="":
    cd infra/cdk && npx cdk synth -c env={{env}} {{ARGS}}
