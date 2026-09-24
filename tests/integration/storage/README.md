# Storage Integration Tests

This directory contains the storage-focused integration tests for the persistence layer adapters.
Self-contained tests stay in `tests/integration/storage/`, while infra-backed PostgreSQL tests live in `tests/integration/storage/infra/`.

## Running Integration Tests

### Prerequisites

Integration tests require Docker.

```bash
# Ensure Docker daemon is running
docker ps

# Run infra-backed storage integration tests
go test -tags=integration -v ./tests/integration/storage/infra/...

# Or using justfile
just test-integration-infra
```

## Test Organization

### Structure
```
tests/integration/storage/
├── README.md                 # This file
├── lifecycle_test.go         # Memory adapter lifecycle tests (self-contained)
└── infra/
    ├── postgres_test.go      # PostgreSQL adapter tests (integration build tag)
    └── thirdparty_service_test.go # PostgreSQL third-party service integration
```

### Memory Adapter Tests (Self-Contained)
Run with standard `go test`:
```bash
go test -v ./tests/integration/storage/...
# or just test-integration
```

Tests:
- `TestMemoryAdapter_FullLifecycle` - Complete workflow
- `TestMemoryAdapter_ConcurrentOperations` - 100+ goroutines
- `TestMemoryAdapter_TimeoutHandling` - Context timeout behavior
- `TestMemoryAdapter_PaginationSupport` - Offset/limit pagination
- `TestMemoryAdapter_ErrorRecovery` - Error handling scenarios

### PostgreSQL Adapter Tests (Infra-Backed, Integration Build Tag)
Requires Docker:
```bash
go test -tags=integration -v ./tests/integration/storage/infra/...
# or just test-integration-infra
```

Tests:
- `TestPostgresAdapter_Initialize_ConnectionFailed` - Invalid connection handling
- `TestPostgresAdapter_HealthCheck_NotInitialized` - Health check validation
- `TestPostgresAdapter_CreateUser_ValidationErrors` - Input validation
- `TestPostgresAdapter_ContextCancellation` - Context cancellation handling
- `TestPostgresAdapter_FullLifecycle_Integration` - Full lifecycle with real PostgreSQL container

## Container Runtime

Locally, tests that require containers skip when Docker or Podman is unavailable. In CI, missing or failed containers fail the infra suites instead of passing without coverage.

## Test Configuration

- `TESTCONTAINERS_RYUK_DISABLED` - Disable resource cleanup (useful for debugging)
  - Set to `true` to keep containers running after test failure

## Troubleshooting

### "Connection refused" or "Cannot connect to Docker"

```bash
# Verify that Docker is running
docker ps

# Retry with verbose output
go test -tags=integration -v ./tests/integration/storage/infra/...
```

### Container image not available
```bash
# Check if postgres:15-alpine image is available
docker images | grep postgres

# Pull image manually if needed
docker pull postgres:15-alpine

# Run tests with verbose output
TESTCONTAINERS_LOGS=true go test -tags=integration -v ./tests/integration/storage/infra/...
```

### Tests hang or timeout
```bash
# Check for orphaned containers
docker ps -a | grep postgres

# Clean up if needed
docker rm -f $(docker ps -aq --filter ancestor=postgres:15-alpine)

# Run tests with debug output
TESTCONTAINERS_LOGS=true go test -tags=integration -v ./tests/integration/storage/infra/...
```

## CI/CD Integration

Provide a Docker daemon to jobs that run infra-backed integration tests:

```bash
go test -tags=integration -v ./tests/integration/storage/infra/...
```

## Performance Notes

Integration tests with real PostgreSQL containers are slower than unit tests:
- Setup time: 2-5 seconds (pulling image, starting container)
- Test execution: 1-10 seconds per test
- Cleanup time: 1-2 seconds

For CI/CD, consider:
- Running integration tests separately from unit tests
- Caching container images to speed up setup
- Running tests in parallel (with appropriate test isolation)

## References

- [testcontainers-go Documentation](https://golang.testcontainers.org/)
- [PostgreSQL Docker Image](https://hub.docker.com/_/postgres)
