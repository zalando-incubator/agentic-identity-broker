#!/bin/bash
# Register mock OAuth2 service with identity broker admin API
# Note: Not using 'set -e' to allow graceful error handling

# Allow override via environment variables for container support
ADMIN_API="${ADMIN_API:-http://localhost:14000/api}"
BROKER_HEALTH_URL="${BROKER_HEALTH_URL:-http://localhost:14000/health}"
MOCK_SERVER_URL="${MOCK_SERVER_URL:-http://localhost:9000}"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "========================================="
echo "Registering Mock OAuth2 Service"
echo "========================================="

# Check broker admin API is ready
echo -n "Checking broker admin API... "
BROKER_READY=false
if curl -s -f "${BROKER_HEALTH_URL}" > /dev/null; then
    echo -e "${GREEN}OK${NC}"
    BROKER_READY=true
else
    echo -e "${RED}FAILED (will retry)${NC}"
    # Retry a few times before giving up
    for i in {1..5}; do
        sleep 1
        if curl -s -f "${BROKER_HEALTH_URL}" > /dev/null; then
            echo -e "${GREEN}✓ Broker ready after retry${NC}"
            BROKER_READY=true
            break
        fi
    done
fi

# Skip registration if broker isn't ready
if [ "$BROKER_READY" != "true" ]; then
    echo "Warning: Skipping registration - broker not ready"
    exit 0  # Exit successfully, just skipping registration
fi

# Register service
echo "Registering mock OAuth2 service..."
SERVICE_RESPONSE=$(curl -s -X POST "${ADMIN_API}/services" \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: admin@example.com" \
  -w "\n%{http_code}" \
  -d "{
    \"display_name\": \"Mock OAuth2 Service (Dev)\",
    \"client_id\": \"mock-oauth2-client-dev\",
    \"client_secret\": \"mock-oauth2-secret-12345\",
    \"issuer_uri\": \"${MOCK_SERVER_URL}\",
    \"discovery\": {\"enable_discovery\": false},
    \"endpoints\": {
      \"token_endpoint\": \"${MOCK_SERVER_URL}/oauth/token\",
      \"authorize_endpoint\": \"${MOCK_SERVER_URL}/oauth/authorize\"
    },
    \"scopes\": [
      {\"scope_value\": \"profile\", \"description\": \"Access user profile\"},
      {\"scope_value\": \"email\", \"description\": \"Access email address\"},
      {\"scope_value\": \"read\", \"description\": \"Read data\"},
      {\"scope_value\": \"write\", \"description\": \"Write data\"}
    ]
  }")

HTTP_CODE=$(echo "$SERVICE_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$SERVICE_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
    echo -e "${GREEN}✓ Mock OAuth2 service registered${NC}"
    SERVICE_ID=$(echo "$RESPONSE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    echo "  Service ID: $SERVICE_ID"
    echo ""
    echo "Next steps:"
    echo "  1. Open: http://localhost:8000/api/third-party/$SERVICE_ID/oauth2/authorize?redirect_uri=%2Fconnections"
    echo "  2. Approve consent on mock OAuth2 page"
    echo "  3. Session established!"
    exit 0
elif [ "$HTTP_CODE" = "409" ]; then
    echo -e "${YELLOW}✓ Mock OAuth2 service already exists (idempotent)${NC}"
    exit 0
else
    echo -e "${RED}✗ Registration failed${NC}"
    echo "  HTTP Status: $HTTP_CODE"
    echo "  Response: $RESPONSE_BODY"
    exit 1
fi
