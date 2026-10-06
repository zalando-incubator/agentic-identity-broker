#!/bin/bash
# Seed sample data for development and testing
# Creates sample agents and third-party services via the admin API

set -e

# Allow override via environment variable for container support
ADMIN_API="${ADMIN_API:-http://localhost:14000/api}"
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "========================================="
echo "Seeding Sample Data via Admin API"
echo "========================================="
echo ""

# Check if admin API is accessible
echo -n "Checking admin API connectivity... "
if curl -s -f "http://localhost:14000/health" > /dev/null; then
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${RED}FAILED${NC}"
    echo "Error: Admin API is not accessible at ${ADMIN_API}"
    echo "Please ensure the identity broker is running with admin API on port 14000"
    exit 1
fi
echo ""

# Create Weather Assistant Agent
echo "Creating Weather Assistant agent..."
AGENT_RESPONSE=$(curl -s -X POST "${ADMIN_API}/agents" \
  -H "Content-Type: application/json" \
  -w "\n%{http_code}" \
  -d '{
    "client_id": "weather-assistant-001",
    "display_name": "Weather Assistant",
    "description": "AI assistant that helps you check weather forecasts and conditions",
    "governance_url": "https://example.com/weather-assistant/governance",
    "user_documentation_url": "https://example.com/weather-assistant/docs",
    "agent_interface_url": "https://example.com/weather-assistant"
  }')

HTTP_CODE=$(echo "$AGENT_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$AGENT_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
    echo -e "${GREEN}✓ Weather Assistant created${NC}"
    AGENT_ID=$(echo "$RESPONSE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    echo "  Agent ID: $AGENT_ID"
elif [ "$HTTP_CODE" = "409" ]; then
    echo -e "${YELLOW}✓ Weather Assistant already exists (idempotent)${NC}"
else
    echo -e "${RED}✗ Failed to create Weather Assistant${NC}"
    echo "  HTTP Status: $HTTP_CODE"
    echo "  Response: $RESPONSE_BODY"
    exit 1
fi
echo ""

# Create Task Manager Agent
echo "Creating Task Manager agent..."
AGENT_RESPONSE=$(curl -s -X POST "${ADMIN_API}/agents" \
  -H "Content-Type: application/json" \
  -w "\n%{http_code}" \
  -d '{
    "client_id": "task-manager-agent",
    "display_name": "Task Manager Pro",
    "description": "Intelligent task management assistant that helps organize your work",
    "governance_url": "https://example.com/task-manager/governance",
    "user_documentation_url": "https://example.com/task-manager/docs"
  }')

HTTP_CODE=$(echo "$AGENT_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$AGENT_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
    echo -e "${GREEN}✓ Task Manager Pro created${NC}"
    AGENT_ID=$(echo "$RESPONSE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    echo "  Agent ID: $AGENT_ID"
elif [ "$HTTP_CODE" = "409" ]; then
    echo -e "${YELLOW}✓ Task Manager Pro already exists (idempotent)${NC}"
else
    echo -e "${RED}✗ Failed to create Task Manager Pro${NC}"
    echo "  HTTP Status: $HTTP_CODE"
    echo "  Response: $RESPONSE_BODY"
    exit 1
fi
echo ""

# Create OAuth2 Test Client Agent (uses upstream-oauth2-client from mock server)
echo "Creating OAuth2 Test Client agent..."
AGENT_RESPONSE=$(curl -s -X POST "${ADMIN_API}/agents" \
  -H "Content-Type: application/json" \
  -w "\n%{http_code}" \
  -d '{
    "client_id": "upstream-oauth2-client",
    "display_name": "OAuth2 Test Client",
    "description": "Test client for OAuth2 authorization flow with upstream mock server on port 9001",
    "governance_url": "https://example.com/oauth2-test/governance",
    "user_documentation_url": "https://example.com/oauth2-test/docs",
    "agent_interface_url": "http://localhost:8000",
    "redirect_uris": ["http://localhost:9002/oauth2/callback"]
  }')

HTTP_CODE=$(echo "$AGENT_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$AGENT_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
    echo -e "${GREEN}✓ OAuth2 Test Client created${NC}"
    AGENT_ID=$(echo "$RESPONSE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    echo "  Agent ID: $AGENT_ID"
elif [ "$HTTP_CODE" = "409" ]; then
    echo -e "${YELLOW}✓ OAuth2 Test Client already exists (idempotent)${NC}"
else
    echo -e "${RED}✗ Failed to create OAuth2 Test Client${NC}"
    echo "  HTTP Status: $HTTP_CODE"
    echo "  Response: $RESPONSE_BODY"
    exit 1
fi
echo ""

# Create Weather API Service
echo "Creating Weather API service..."
SERVICE_RESPONSE=$(curl -s -X POST "${ADMIN_API}/services" \
  -H "Content-Type: application/json" \
  -w "\n%{http_code}" \
  -d '{
    "display_name": "Weather API",
    "client_id": "weather-api-client",
    "client_secret": "weather-secret-12345",
    "issuer_uri": "https://auth.weather-api.example.com",
    "discovery": {
      "enable_discovery": false
    },
    "endpoints": {
      "token_endpoint": "https://auth.weather-api.example.com/oauth/token",
      "authorize_endpoint": "https://auth.weather-api.example.com/oauth/authorize"
    },
    "scopes": [
      {
        "scope_value": "weather.read",
        "description": "Read current weather conditions"
      },
      {
        "scope_value": "weather.forecast",
        "description": "Access weather forecasts"
      },
      {
        "scope_value": "weather.alerts",
        "description": "Receive weather alerts"
      }
    ]
  }')

HTTP_CODE=$(echo "$SERVICE_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$SERVICE_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
    echo -e "${GREEN}✓ Weather API service created${NC}"
    SERVICE_ID=$(echo "$RESPONSE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    echo "  Service ID: $SERVICE_ID"
elif [ "$HTTP_CODE" = "409" ]; then
    echo -e "${YELLOW}✓ Weather API service already exists (idempotent)${NC}"
else
    echo -e "${RED}✗ Failed to create Weather API service${NC}"
    echo "  HTTP Status: $HTTP_CODE"
    echo "  Response: $RESPONSE_BODY"
    exit 1
fi
echo ""

# Create Calendar Service
echo "Creating Calendar API service..."
SERVICE_RESPONSE=$(curl -s -X POST "${ADMIN_API}/services" \
  -H "Content-Type: application/json" \
  -w "\n%{http_code}" \
  -d '{
    "display_name": "Calendar API",
    "client_id": "calendar-api-client",
    "client_secret": "calendar-secret-67890",
    "issuer_uri": "https://auth.calendar-api.example.com",
    "discovery": {
      "enable_discovery": false
    },
    "endpoints": {
      "token_endpoint": "https://auth.calendar-api.example.com/oauth/token",
      "authorize_endpoint": "https://auth.calendar-api.example.com/oauth/authorize"
    },
    "scopes": [
      {
        "scope_value": "calendar.read",
        "description": "Read calendar events"
      },
      {
        "scope_value": "calendar.write",
        "description": "Create and modify calendar events"
      },
      {
        "scope_value": "calendar.events.readonly",
        "description": "Read-only access to calendar events"
      }
    ]
  }')

HTTP_CODE=$(echo "$SERVICE_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$SERVICE_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
    echo -e "${GREEN}✓ Calendar API service created${NC}"
    SERVICE_ID=$(echo "$RESPONSE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    echo "  Service ID: $SERVICE_ID"
elif [ "$HTTP_CODE" = "409" ]; then
    echo -e "${YELLOW}✓ Calendar API service already exists (idempotent)${NC}"
else
    echo -e "${RED}✗ Failed to create Calendar API service${NC}"
    echo "  HTTP Status: $HTTP_CODE"
    echo "  Response: $RESPONSE_BODY"
    exit 1
fi
echo ""

# Create Email Service
echo "Creating Email API service..."
SERVICE_RESPONSE=$(curl -s -X POST "${ADMIN_API}/services" \
  -H "Content-Type: application/json" \
  -w "\n%{http_code}" \
  -d '{
    "display_name": "Email API",
    "client_id": "email-api-client",
    "client_secret": "email-secret-abcde",
    "issuer_uri": "https://auth.email-api.example.com",
    "discovery": {
      "enable_discovery": false
    },
    "endpoints": {
      "token_endpoint": "https://auth.email-api.example.com/oauth/token",
      "authorize_endpoint": "https://auth.email-api.example.com/oauth/authorize"
    },
    "scopes": [
      {
        "scope_value": "email.read",
        "description": "Read email messages"
      },
      {
        "scope_value": "email.send",
        "description": "Send email messages"
      },
      {
        "scope_value": "email.compose",
        "description": "Compose draft emails"
      }
    ]
  }')

HTTP_CODE=$(echo "$SERVICE_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$SERVICE_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
    echo -e "${GREEN}✓ Email API service created${NC}"
    SERVICE_ID=$(echo "$RESPONSE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    echo "  Service ID: $SERVICE_ID"
elif [ "$HTTP_CODE" = "409" ]; then
    echo -e "${YELLOW}✓ Email API service already exists (idempotent)${NC}"
else
    echo -e "${RED}✗ Failed to create Email API service${NC}"
    echo "  HTTP Status: $HTTP_CODE"
    echo "  Response: $RESPONSE_BODY"
    exit 1
fi
echo ""

echo "========================================="
echo "Sample Data Seeding Complete"
echo "========================================="
echo ""
echo "Summary:"
echo "  Agents created: 3"
echo "    - Weather Assistant"
echo "    - Task Manager Pro"
echo "    - OAuth2 Test Client (upstream-oauth2-client)"
echo ""
echo "  Services created: 3"
echo "    - Weather API"
echo "    - Calendar API"
echo "    - Email API"
echo ""
echo "Testing OAuth2 Authorization Flow:"
echo "  1. Start upstream OAuth2 mock server:"
echo "     just mock-upstream-oauth2-start"
echo "  2. Start identity broker:"
echo "     just dev"
echo "  3. Test OAuth2 flow with OAuth2 Test Client (port 9001)"
echo ""
echo "You can now test the consent management UI with this sample data."
echo ""
