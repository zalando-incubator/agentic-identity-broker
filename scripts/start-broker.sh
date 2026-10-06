#!/bin/bash
# Broker startup wrapper - Auto-seeds data in background after broker starts
# This runs every time Air rebuilds, allowing auto-seed on code changes

# Color codes
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# PID file for tracking seed job (works in both Docker and native dev)
PID_FILE="./tmp/.seed_job.pid"

# Cleanup function for seed job PID file
cleanup_seed_pid() {
    rm -f "$PID_FILE"
}
trap cleanup_seed_pid EXIT

# Function to apply seed data (runs in background)
apply_seed_data() {
    # Wait for broker to be fully ready (health check + API responding)
    echo -e "${YELLOW}[Broker Wrapper]${NC} Waiting for broker API to be ready..."
    for i in {1..40}; do
        if curl -s http://localhost:14000/health > /dev/null 2>&1 && \
           curl -s http://localhost:14000/api/services > /dev/null 2>&1; then
            break
        fi
        sleep 0.5
    done

    # Set environment variables for seed scripts
    export ADMIN_API="http://localhost:14000/api"
    export BROKER_HEALTH_URL="http://localhost:14000/health"

    # Detect if running in Docker/Podman vs local development
    # Token endpoint: service-to-service communication (use service name in Docker, localhost in local)
    # Authorize endpoint: browser redirect (always use localhost)
    # Try to resolve service name - if successful, we're in Docker/Podman, otherwise we're in local dev
    if getent hosts third-party-oauth2 > /dev/null 2>&1; then
        # Running in Docker/Podman Compose - use service name for token endpoint
        export MOCK_SERVER_TOKEN_URL="http://third-party-oauth2:9000"
        echo -e "${YELLOW}[Broker Wrapper]${NC} Running in container - using service name for token endpoint"
    else
        # Running in local development - use localhost for both
        export MOCK_SERVER_TOKEN_URL="http://localhost:9000"
        echo -e "${YELLOW}[Broker Wrapper]${NC} Running in local development - using localhost for endpoints"
    fi
    # Authorize endpoint always uses localhost (accessed by browser on user's machine)
    export MOCK_SERVER_AUTHORIZE_URL="http://localhost:9000"

    echo -e "${YELLOW}[Broker Wrapper]${NC} Seeding data..."
    echo -e "${YELLOW}[Broker Wrapper]${NC} Token endpoint: ${MOCK_SERVER_TOKEN_URL}"
    echo -e "${YELLOW}[Broker Wrapper]${NC} Authorize endpoint: ${MOCK_SERVER_AUTHORIZE_URL}"

    # Run combined setup script with improved error visibility
    # This creates agents, services, AND service requirements
    if [ -f "./scripts/setup-dev-data.sh" ]; then
        # Capture output in temp file for error reporting
        SETUP_LOG=$(mktemp)
        trap "rm -f $SETUP_LOG" RETURN

        if bash ./scripts/setup-dev-data.sh > "$SETUP_LOG" 2>&1; then
            echo -e "${GREEN}[Broker Wrapper]${NC} setup-dev-data.sh completed"
            # Show summary lines only on success
            tail -5 "$SETUP_LOG" | grep -E "(Agents|Services|requirements)" || true
        else
            echo -e "${RED}[Broker Wrapper]${NC} setup-dev-data.sh FAILED - showing full output:"
            echo "----------------------------------------"
            cat "$SETUP_LOG"
            echo "----------------------------------------"
        fi
    fi

    echo -e "${GREEN}[Broker Wrapper]${NC} Auto-seed complete"
}

# Only auto-seed if setup script is present (indicates docker-compose environment)
if [ -f "./scripts/setup-dev-data.sh" ]; then
    echo -e "${YELLOW}[Broker Wrapper]${NC} Auto-seed enabled - seeding will run in background"

    # Kill any existing seed job before starting new one
    if [ -f "$PID_FILE" ]; then
        OLD_PID=$(cat "$PID_FILE")
        if kill -0 "$OLD_PID" 2>/dev/null; then
            echo -e "${YELLOW}[Broker Wrapper]${NC} Terminating previous seed job (PID: $OLD_PID)"
            kill "$OLD_PID" 2>/dev/null
            sleep 1
            # Force kill if still running
            kill -9 "$OLD_PID" 2>/dev/null || true
        fi
        rm -f "$PID_FILE"
    fi

    # Start background seed job (runs after broker starts)
    apply_seed_data &
    SEED_JOB_PID=$!
    echo "$SEED_JOB_PID" > "$PID_FILE"
fi

# Exec the actual broker binary (replaces this script's process)
# The seed job continues running in background after the broker starts
exec ./tmp/agentic-identity-broker
