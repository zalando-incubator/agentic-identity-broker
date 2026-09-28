#!/usr/bin/env bash
# Validate Helm chart deployment in Kind cluster
# This script serves as infrastructure E2E testing for the Helm chart

set -e
set -o pipefail

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
CLUSTER_NAME="${CLUSTER_NAME:-helm-test}"
RELEASE_NAME="${RELEASE_NAME:-broker}"
CHART_PATH="./charts/agentic-identity-broker"
NAMESPACE="${NAMESPACE:-default}"
TIMEOUT="5m"

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up Kind cluster...${NC}"
    kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true
}

# Trap cleanup on exit
trap cleanup EXIT

# Print section header
print_header() {
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}$1${NC}"
    echo -e "${GREEN}========================================${NC}"
}

# Print success message
print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

# Print error message and exit
print_error() {
    echo -e "${RED}✗ $1${NC}"
    exit 1
}

# Check prerequisites
print_header "Checking Prerequisites"
for cmd in kind helm kubectl docker; do
    if ! command -v "$cmd" &> /dev/null; then
        print_error "$cmd is not installed. Please install it first."
    fi
    print_success "$cmd is installed"
done

# Validate Helm chart syntax
print_header "Validating Helm Chart Syntax"
helm lint "${CHART_PATH}" || print_error "Helm lint failed"
print_success "Helm chart syntax is valid"

# Test template rendering
print_header "Testing Template Rendering"
helm template "${RELEASE_NAME}" "${CHART_PATH}" > /dev/null || print_error "Template rendering failed"
print_success "Templates render successfully"

# Create Kind cluster
print_header "Creating Kind Cluster"
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster ${CLUSTER_NAME} already exists, deleting..."
    kind delete cluster --name "${CLUSTER_NAME}"
fi

kind create cluster --name "${CLUSTER_NAME}" --wait "${TIMEOUT}" || print_error "Failed to create Kind cluster"
print_success "Kind cluster created"

# Wait for cluster to be ready
kubectl cluster-info --context "kind-${CLUSTER_NAME}" || print_error "Cannot connect to cluster"
print_success "Cluster is reachable"

# Load Docker images into Kind cluster
print_header "Loading Docker Images into Kind"

# Load broker image
BROKER_IMAGE="localhost/agentic-identity-broker:0.1.0"
if docker image inspect "${BROKER_IMAGE}" >/dev/null 2>&1; then
    echo "Loading broker image ${BROKER_IMAGE} from docker..."
    kind load docker-image "${BROKER_IMAGE}" --name "${CLUSTER_NAME}" || print_error "Failed to load broker image from docker"
else
    print_error "Broker image ${BROKER_IMAGE} not found. Build it first with: docker build -t ${BROKER_IMAGE} -f build/docker/Dockerfile ."
fi
print_success "Broker image loaded into Kind cluster"

# Load migrate image
MIGRATE_IMAGE="localhost/agentic-identity-broker-migrate:0.1.0"
if docker image inspect "${MIGRATE_IMAGE}" >/dev/null 2>&1; then
    echo "Loading migrate image ${MIGRATE_IMAGE} from docker..."
    kind load docker-image "${MIGRATE_IMAGE}" --name "${CLUSTER_NAME}" || print_error "Failed to load migrate image from docker"
else
    print_error "Migrate image ${MIGRATE_IMAGE} not found. Build it first with: docker build -t ${MIGRATE_IMAGE} -f build/docker/Dockerfile.migrate ."
fi
print_success "Migrate image loaded into Kind cluster"

# Generate JWE signing key for testing
JWE_SIGNING_KEY=$(openssl rand -base64 32)

# Test 1: Deploy with default values (in-memory storage)
print_header "Test 1: Deploy with In-Memory Storage (Default)"
helm install "${RELEASE_NAME}" "${CHART_PATH}" \
    --namespace "${NAMESPACE}" \
    --set image.pullPolicy=Never \
    --set image.repository=localhost/agentic-identity-broker \
    --set image.tag=0.1.0 \
    --set migration.image.pullPolicy=Never \
    --set migration.image.repository=localhost/agentic-identity-broker-migrate \
    --set migration.image.tag=0.1.0 \
    --set broker.thirdPartyOauth2.jweSigningKey="${JWE_SIGNING_KEY}" \
    --wait \
    --timeout "${TIMEOUT}" || print_error "Helm install failed (in-memory mode)"
print_success "Chart installed successfully"

# Verify deployment
kubectl get deployment "${RELEASE_NAME}-agentic-identity-broker" -n "${NAMESPACE}" || print_error "Deployment not found"
kubectl wait --for=condition=available deployment/"${RELEASE_NAME}-agentic-identity-broker" -n "${NAMESPACE}" --timeout=60s || print_error "Deployment did not become available"
print_success "Deployment is available"

# Verify service
kubectl get service "${RELEASE_NAME}-agentic-identity-broker" -n "${NAMESPACE}" || print_error "Service not found"
print_success "Service created"

# Run Helm tests
print_header "Running Helm Tests"
helm test "${RELEASE_NAME}" -n "${NAMESPACE}" || print_error "Helm test failed"
print_success "Helm tests passed"

# Verify health endpoint via port-forward
print_header "Verifying Health Endpoint"
kubectl port-forward -n "${NAMESPACE}" "svc/${RELEASE_NAME}-agentic-identity-broker" 8000:8000 &
PORT_FORWARD_PID=$!
sleep 3

# Test health endpoint
if curl -f -s http://localhost:8000/health > /dev/null; then
    print_success "Health endpoint is accessible"
else
    kill ${PORT_FORWARD_PID} 2>/dev/null || true
    print_error "Health endpoint is not accessible"
fi

# Cleanup port-forward
kill ${PORT_FORWARD_PID} 2>/dev/null || true

# Uninstall release
print_header "Uninstalling Release"
helm uninstall "${RELEASE_NAME}" -n "${NAMESPACE}" || print_error "Helm uninstall failed"
print_success "Release uninstalled"

# Test 2: Verify template with PostgreSQL external mode
print_header "Test 2: Template Rendering with External PostgreSQL"
helm template "${RELEASE_NAME}" "${CHART_PATH}" \
    --set storage.type=postgres \
    --set postgresql.external.enabled=true \
    --set postgresql.external.host=postgres.database.svc.cluster.local \
    --set postgresql.external.database=broker \
    --set postgresql.external.migrationSecretName=broker-migration \
    --set postgresql.external.brokerSecretName=broker-db \
    > /dev/null || print_error "Template rendering failed (external PostgreSQL)"
print_success "Templates render with external PostgreSQL configuration"

# Test 3: Verify template with Zalando operator mode
print_header "Test 3: Template Rendering with Zalando PostgreSQL Operator"
helm template "${RELEASE_NAME}" "${CHART_PATH}" \
    --set storage.type=postgres \
    --set postgresql.operator.enabled=true \
    --set postgresql.operator.teamId=test-team \
    > /dev/null || print_error "Template rendering failed (Zalando operator)"
print_success "Templates render with Zalando operator configuration"

# Test 4: Verify Ingress templates
print_header "Test 4: Template Rendering with Ingress Enabled"
helm template "${RELEASE_NAME}" "${CHART_PATH}" \
    --set ingress.enduser.enabled=true \
    --set ingress.enduser.hosts[0].host=broker.example.com \
    --set ingress.enduser.hosts[0].paths[0].path=/ \
    --set ingress.enduser.hosts[0].paths[0].pathType=Prefix \
    --set ingress.admin.enabled=true \
    --set ingress.admin.hosts[0].host=broker-admin.example.com \
    --set ingress.admin.hosts[0].paths[0].path=/ \
    --set ingress.admin.hosts[0].paths[0].pathType=Prefix \
    > /dev/null || print_error "Template rendering failed (Ingress)"
print_success "Templates render with Ingress configuration"

# Test 4a: Verify grants ConfigMap is rendered by default (external PostgreSQL)
print_header "Test 4a: Grants ConfigMap Rendered with External PostgreSQL"
GRANTS_TEMPLATE=$(helm template "${RELEASE_NAME}" "${CHART_PATH}" \
    --set storage.type=postgres \
    --set postgresql.external.enabled=true \
    --set postgresql.external.host=postgres.database.svc.cluster.local \
    --set postgresql.external.database=broker \
    --set postgresql.external.migrationSecretName=broker-migration \
    --set postgresql.external.brokerSecretName=broker-db)
echo "${GRANTS_TEMPLATE}" | grep -q "grants.sql" || print_error "Grants ConfigMap not found in template output"
echo "${GRANTS_TEMPLATE}" | grep -q "apply-grants" || print_error "apply-grants container not found in migration Job"
echo "${GRANTS_TEMPLATE}" | grep -q "BROKER_USERNAME" || print_error "BROKER_USERNAME env var not found in migration Job"
print_success "Grants ConfigMap and apply-grants container are present (external PostgreSQL)"

# Test 4b: Verify grants ConfigMap is rendered by default (Zalando operator)
print_header "Test 4b: Grants ConfigMap Rendered with Zalando Operator"
GRANTS_TEMPLATE=$(helm template "${RELEASE_NAME}" "${CHART_PATH}" \
    --set storage.type=postgres \
    --set postgresql.operator.enabled=true \
    --set postgresql.operator.teamId=test-team)
echo "${GRANTS_TEMPLATE}" | grep -q "grants.sql" || print_error "Grants ConfigMap not found in template output"
echo "${GRANTS_TEMPLATE}" | grep -q "apply-grants" || print_error "apply-grants container not found in migration Job"
print_success "Grants ConfigMap and apply-grants container are present (Zalando operator)"

# Test 4c: Verify custom grants SQL is applied
print_header "Test 4c: Custom Grants SQL Configuration"
CUSTOM_TEMPLATE=$(helm template "${RELEASE_NAME}" "${CHART_PATH}" \
    --set storage.type=postgres \
    --set postgresql.external.enabled=true \
    --set postgresql.external.host=postgres.database.svc.cluster.local \
    --set postgresql.external.database=broker \
    --set postgresql.external.migrationSecretName=broker-migration \
    --set postgresql.external.brokerSecretName=broker-db \
    --set "migration.grants.sql=GRANT SELECT ON ALL TABLES IN SCHEMA public TO :\"broker_user\";")
echo "${CUSTOM_TEMPLATE}" | grep -q "GRANT SELECT ON ALL TABLES" || print_error "Custom grants SQL not found in ConfigMap"
print_success "Custom grants SQL is applied correctly"

# Test 4d: Verify grants can be disabled
print_header "Test 4d: Grants Disabled Configuration"
NO_GRANTS_TEMPLATE=$(helm template "${RELEASE_NAME}" "${CHART_PATH}" \
    --set storage.type=postgres \
    --set postgresql.external.enabled=true \
    --set postgresql.external.host=postgres.database.svc.cluster.local \
    --set postgresql.external.database=broker \
    --set postgresql.external.migrationSecretName=broker-migration \
    --set postgresql.external.brokerSecretName=broker-db \
    --set migration.grants.enabled=false)
echo "${NO_GRANTS_TEMPLATE}" | grep -q "grants.sql" && print_error "Grants ConfigMap should not be present when disabled"
echo "${NO_GRANTS_TEMPLATE}" | grep -q "apply-grants" && print_error "apply-grants container should not be present when disabled"
print_success "Grants are correctly disabled when migration.grants.enabled=false"

# Test 5: Deploy with Zalando PostgreSQL Operator
print_header "Test 5: Deploy with Zalando PostgreSQL Operator"

# Install Zalando PostgreSQL Operator CRDs first
echo "Installing Zalando PostgreSQL Operator CRDs..."
kubectl apply -f https://raw.githubusercontent.com/zalando/postgres-operator/master/manifests/postgresql.crd.yaml || print_error "Failed to install PostgreSQL CRD"
kubectl apply -f https://raw.githubusercontent.com/zalando/postgres-operator/master/manifests/operatorconfiguration.crd.yaml || print_error "Failed to install OperatorConfiguration CRD"
print_success "Zalando operator CRDs installed"

# Install Zalando PostgreSQL Operator
echo "Installing Zalando PostgreSQL Operator..."
kubectl apply -f https://raw.githubusercontent.com/zalando/postgres-operator/master/manifests/configmap.yaml || print_error "Failed to apply ConfigMap"
kubectl apply -f https://raw.githubusercontent.com/zalando/postgres-operator/master/manifests/operator-service-account-rbac.yaml || print_error "Failed to apply RBAC"
kubectl apply -f https://raw.githubusercontent.com/zalando/postgres-operator/master/manifests/postgres-operator.yaml || print_error "Failed to apply operator deployment"
print_success "Zalando operator manifests applied"

# Wait for operator to be ready
echo "Waiting for operator to be ready (this may take 1-2 minutes)..."
kubectl wait --for=condition=available deployment/postgres-operator -n default --timeout=180s || print_error "Operator did not become ready"
print_success "Zalando operator is ready"

# Deploy with Zalando operator enabled
echo "Deploying Helm chart with Zalando operator mode..."
helm install "${RELEASE_NAME}-postgres" "${CHART_PATH}" \
    --namespace "${NAMESPACE}" \
    --set image.pullPolicy=Never \
    --set image.repository=localhost/agentic-identity-broker \
    --set image.tag=0.1.0 \
    --set migration.image.pullPolicy=Never \
    --set migration.image.repository=localhost/agentic-identity-broker-migrate \
    --set migration.image.tag=0.1.0 \
    --set storage.type=postgres \
    --set postgresql.operator.enabled=true \
    --set postgresql.operator.teamId=test-team \
    --set postgresql.operator.volume.size=1Gi \
    --set postgresql.operator.numberOfInstances=1 \
    --set broker.thirdPartyOauth2.jweSigningKey="${JWE_SIGNING_KEY}" \
    --wait \
    --timeout "${TIMEOUT}" || print_error "Helm install failed (Zalando operator mode)"
print_success "Chart installed with Zalando operator"

# Wait for PostgreSQL cluster to be ready
# The postgresql resource name is: {teamId}-{release-name}-agentic-identity-broker
PG_CLUSTER_NAME="test-team-${RELEASE_NAME}-postgres-agentic-identity-broker"
echo "Waiting for PostgreSQL cluster ${PG_CLUSTER_NAME} to be ready..."
kubectl wait --for=jsonpath='{.status.PostgresClusterStatus}'=Running postgresql/"${PG_CLUSTER_NAME}" -n "${NAMESPACE}" --timeout=300s || print_error "PostgreSQL cluster did not become ready"
print_success "PostgreSQL cluster is running"

# Verify migration Job completed
print_header "Verifying Migration Job"
kubectl wait --for=condition=complete job/"${RELEASE_NAME}-postgres-agentic-identity-broker-migrate" -n "${NAMESPACE}" --timeout=120s || print_error "Migration Job did not complete"
print_success "Migration Job completed successfully"

# Check migration logs
echo "Migration Job logs:"
kubectl logs job/"${RELEASE_NAME}-postgres-agentic-identity-broker-migrate" -n "${NAMESPACE}" || true

# Verify broker deployment
kubectl get deployment "${RELEASE_NAME}-postgres-agentic-identity-broker" -n "${NAMESPACE}" || print_error "Deployment not found"
kubectl wait --for=condition=available deployment/"${RELEASE_NAME}-postgres-agentic-identity-broker" -n "${NAMESPACE}" --timeout=120s || print_error "Deployment did not become available"
print_success "Broker deployment is available with PostgreSQL backend"

# Verify broker logs show PostgreSQL connection
echo "Checking broker logs for PostgreSQL connection..."
kubectl logs deployment/"${RELEASE_NAME}-postgres-agentic-identity-broker" -n "${NAMESPACE}" --tail=20 || true

# Verify health endpoint via port-forward
print_header "Verifying Health Endpoint (PostgreSQL Backend)"
kubectl port-forward -n "${NAMESPACE}" "svc/${RELEASE_NAME}-postgres-agentic-identity-broker" 8001:8000 &
PORT_FORWARD_PID=$!
sleep 5

# Test health endpoint
if curl -f -s http://localhost:8001/health > /dev/null; then
    print_success "Health endpoint is accessible with PostgreSQL backend"
else
    kill ${PORT_FORWARD_PID} 2>/dev/null || true
    print_error "Health endpoint is not accessible with PostgreSQL backend"
fi

# Cleanup port-forward
kill ${PORT_FORWARD_PID} 2>/dev/null || true

# Uninstall PostgreSQL release
print_header "Uninstalling PostgreSQL Release"
helm uninstall "${RELEASE_NAME}-postgres" -n "${NAMESPACE}" || print_error "Helm uninstall failed (PostgreSQL release)"
print_success "PostgreSQL release uninstalled"

# Cleanup PostgreSQL CR
echo "Cleaning up PostgreSQL CR..."
kubectl delete postgresql "${PG_CLUSTER_NAME}" -n "${NAMESPACE}" --timeout=60s || true
print_success "PostgreSQL CR deleted"

# Final summary
print_header "Validation Complete"
echo -e "${GREEN}All tests passed successfully!${NC}"
echo ""
echo "Validated:"
echo "  ✓ Helm chart syntax (helm lint)"
echo "  ✓ Template rendering (helm template)"
echo "  ✓ Kind cluster deployment (in-memory)"
echo "  ✓ Deployment availability"
echo "  ✓ Service creation"
echo "  ✓ Helm tests"
echo "  ✓ Health endpoint (in-memory)"
echo "  ✓ External PostgreSQL configuration (template only)"
echo "  ✓ Grants ConfigMap and apply-grants container (external PostgreSQL)"
echo "  ✓ Grants ConfigMap and apply-grants container (Zalando operator)"
echo "  ✓ Custom grants SQL configuration"
echo "  ✓ Grants disabled configuration"
echo "  ✓ Zalando operator deployment (full E2E)"
echo "  ✓ PostgreSQL cluster provisioning"
echo "  ✓ Database migration Job execution"
echo "  ✓ Health endpoint (PostgreSQL backend)"
echo "  ✓ Ingress configuration (template only)"
echo ""
