#!/bin/bash
# Combined setup script: Register mock OAuth2 service AND seed sample data
# Creates agents, third-party services, and permission sets for testing optional service toggles

set -e

# Allow override via environment variables for container support
ADMIN_API="${ADMIN_API:-http://localhost:14000/api}"
BROKER_HEALTH_URL="${BROKER_HEALTH_URL:-http://localhost:14000/health}"
# Separate token and authorize endpoints to support service names in Docker Compose
MOCK_SERVER_TOKEN_URL="${MOCK_SERVER_TOKEN_URL:-http://localhost:9000}"
MOCK_SERVER_AUTHORIZE_URL="${MOCK_SERVER_AUTHORIZE_URL:-http://localhost:9000}"
# CIMD mock server hostname (container DNS name in Docker Compose, localhost otherwise)
CIMD_SERVER_HOST="${CIMD_SERVER_HOST:-cimd-mock}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "========================================="
echo "Setting Up Development Environment"
echo "========================================="
echo ""

# Check if admin API is accessible
echo -n "Checking admin API connectivity... "
if curl -s -f "${BROKER_HEALTH_URL}" >/dev/null; then
	echo -e "${GREEN}OK${NC}"
else
	echo -e "${RED}FAILED${NC}"
	echo "Error: Admin API is not accessible at ${ADMIN_API}"
	echo "Please ensure the identity broker is running with admin API on port 14000"
	exit 1
fi
echo ""

# Helper: look up entity ID from a list endpoint by matching a field value.
# Uses jq when available (Docker container), falls back to grep for host environments.
# Usage: lookup_id_by_field <endpoint> <field> <value>
# Example: lookup_id_by_field "services" "client_id" "weather-api-client"
lookup_id_by_field() {
	local endpoint="$1"
	local field="$2"
	local value="$3"
	local response
	response=$(curl -s "${ADMIN_API}/${endpoint}")
	if command -v jq >/dev/null 2>&1; then
		echo "$response" | jq -r --arg v "$value" '.items[] | select(.'"$field"' == $v) | .id' 2>/dev/null | head -1
	else
		# Fallback: requires python3 (host environments)
		echo "$response" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    for item in data.get('items', []):
        if item.get('$field') == '$value':
            print(item['id'])
            sys.exit(0)
except Exception:
    pass
sys.exit(1)
" 2>/dev/null
	fi
}

# ==========================================
# Step 1: Register Mock OAuth2 Service
# ==========================================
echo "========================================="
echo "Step 1: Registering Mock OAuth2 Service"
echo "========================================="
echo ""

echo "Registering mock OAuth2 service..."
SERVICE_RESPONSE=$(curl -s -X POST "${ADMIN_API}/services" \
	-H "Content-Type: application/json" \
	-H "X-Remote-User: admin@example.com" \
	-w "\n%{http_code}" \
	-d "{
    \"display_name\": \"Mock OAuth2 Service (Dev)\",
    \"client_id\": \"mock-oauth2-client-dev\",
    \"client_secret\": \"mock-oauth2-secret-12345\",
    \"issuer_uri\": \"http://localhost:9000\",
    \"discovery\": {\"enable_discovery\": false},
    \"endpoints\": {
      \"token_endpoint\": \"${MOCK_SERVER_TOKEN_URL}/oauth/token\",
      \"authorize_endpoint\": \"${MOCK_SERVER_AUTHORIZE_URL}/oauth/authorize\"
    },
    \"scopes\": [
      {\"scope_value\": \"profile\", \"description\": \"Access user profile\"},
      {\"scope_value\": \"email\", \"description\": \"Access email address\"},
      {\"scope_value\": \"read\", \"description\": \"Read data\"},
      {\"scope_value\": \"write\", \"description\": \"Write data\"}
    ],
    \"protected_resources\": [\"http://agentgateway:4000/mcp\"]
  }")

HTTP_CODE=$(echo "$SERVICE_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$SERVICE_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
	MOCK_SERVICE_ID=$(echo "$RESPONSE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
	echo -e "${GREEN}✓ Mock OAuth2 service registered (ID: $MOCK_SERVICE_ID)${NC}"
elif [ "$HTTP_CODE" = "409" ]; then
	MOCK_SERVICE_ID=$(lookup_id_by_field "services" "client_id" "mock-oauth2-client-dev")
	echo -e "${YELLOW}✓ Mock OAuth2 service already exists (ID: $MOCK_SERVICE_ID)${NC}"
else
	echo -e "${RED}✗ Failed to register Mock OAuth2 service${NC}"
	echo "  HTTP Status: $HTTP_CODE"
	echo "  Response: $RESPONSE_BODY"
	exit 1
fi
echo ""

# ==========================================
# Step 2: Create Third-Party Services
# ==========================================
echo "========================================="
echo "Step 2: Creating Third-Party Services"
echo "========================================="
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
    "discovery": {"enable_discovery": false},
    "endpoints": {
      "token_endpoint": "https://auth.weather-api.example.com/oauth/token",
      "authorize_endpoint": "https://auth.weather-api.example.com/oauth/authorize"
    },
    "scopes": [
      {"scope_value": "weather.read",     "description": "Read current weather conditions"},
      {"scope_value": "weather.forecast", "description": "Access weather forecasts"},
      {"scope_value": "weather.alerts",   "description": "Receive weather alerts"}
    ]
  }')

HTTP_CODE=$(echo "$SERVICE_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$SERVICE_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
	WEATHER_SERVICE_ID=$(echo "$RESPONSE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
	echo -e "${GREEN}✓ Weather API service created (ID: $WEATHER_SERVICE_ID)${NC}"
elif [ "$HTTP_CODE" = "409" ]; then
	WEATHER_SERVICE_ID=$(lookup_id_by_field "services" "client_id" "weather-api-client")
	echo -e "${YELLOW}✓ Weather API service already exists (ID: $WEATHER_SERVICE_ID)${NC}"
else
	echo -e "${RED}✗ Failed to create Weather API service${NC}"
	echo "  HTTP Status: $HTTP_CODE"
	echo "  Response: $RESPONSE_BODY"
	exit 1
fi
echo ""

# Create Calendar API Service
echo "Creating Calendar API service..."
SERVICE_RESPONSE=$(curl -s -X POST "${ADMIN_API}/services" \
	-H "Content-Type: application/json" \
	-w "\n%{http_code}" \
	-d '{
    "display_name": "Calendar API",
    "client_id": "calendar-api-client",
    "client_secret": "calendar-secret-67890",
    "issuer_uri": "https://auth.calendar-api.example.com",
    "discovery": {"enable_discovery": false},
    "endpoints": {
      "token_endpoint": "https://auth.calendar-api.example.com/oauth/token",
      "authorize_endpoint": "https://auth.calendar-api.example.com/oauth/authorize"
    },
    "scopes": [
      {"scope_value": "calendar.read",            "description": "Read calendar events"},
      {"scope_value": "calendar.write",           "description": "Create and modify calendar events"},
      {"scope_value": "calendar.events.readonly", "description": "Read-only access to calendar events"}
    ]
  }')

HTTP_CODE=$(echo "$SERVICE_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$SERVICE_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
	CALENDAR_SERVICE_ID=$(echo "$RESPONSE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
	echo -e "${GREEN}✓ Calendar API service created (ID: $CALENDAR_SERVICE_ID)${NC}"
elif [ "$HTTP_CODE" = "409" ]; then
	CALENDAR_SERVICE_ID=$(lookup_id_by_field "services" "client_id" "calendar-api-client")
	echo -e "${YELLOW}✓ Calendar API service already exists (ID: $CALENDAR_SERVICE_ID)${NC}"
else
	echo -e "${RED}✗ Failed to create Calendar API service${NC}"
	echo "  HTTP Status: $HTTP_CODE"
	echo "  Response: $RESPONSE_BODY"
	exit 1
fi
echo ""

# Create Email API Service
echo "Creating Email API service..."
SERVICE_RESPONSE=$(curl -s -X POST "${ADMIN_API}/services" \
	-H "Content-Type: application/json" \
	-w "\n%{http_code}" \
	-d '{
    "display_name": "Email API",
    "client_id": "email-api-client",
    "client_secret": "email-secret-abcde",
    "issuer_uri": "https://auth.email-api.example.com",
    "discovery": {"enable_discovery": false},
    "endpoints": {
      "token_endpoint": "https://auth.email-api.example.com/oauth/token",
      "authorize_endpoint": "https://auth.email-api.example.com/oauth/authorize"
    },
    "scopes": [
      {"scope_value": "email.read",    "description": "Read email messages"},
      {"scope_value": "email.send",    "description": "Send email messages"},
      {"scope_value": "email.compose", "description": "Compose draft emails"}
    ]
  }')

HTTP_CODE=$(echo "$SERVICE_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$SERVICE_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
	echo -e "${GREEN}✓ Email API service created${NC}"
elif [ "$HTTP_CODE" = "409" ]; then
	echo -e "${YELLOW}✓ Email API service already exists${NC}"
else
	echo -e "${RED}✗ Failed to create Email API service${NC}"
	echo "  HTTP Status: $HTTP_CODE"
	exit 1
fi
echo ""

# ==========================================
# Step 3: Create Permission Sets
# Used by simple agents (Step 4) and the permission-set test agent (Step 5).
# Services must exist (Step 2) before permission sets that reference them.
# ==========================================
echo "========================================="
echo "Step 3: Creating Permission Sets"
echo "========================================="
echo ""

# PS1: "Identity Access" (mandatory) — covers profile + email from Mock OAuth2
echo "Creating 'Identity Access' permission set (mandatory)..."
PS_RESPONSE=$(curl -s -X POST "${ADMIN_API}/permission-sets" \
	-H "Content-Type: application/json" \
	-w "\n%{http_code}" \
	-d "{
    \"name\": \"Identity Access\",
    \"description\": \"Access your profile and email address from the identity provider\",
    \"service_scopes\": [
      {
        \"service_id\": \"${MOCK_SERVICE_ID}\",
        \"scopes\": [\"profile\", \"email\"]
      }
    ]
  }")

PS_HTTP=$(echo "$PS_RESPONSE" | tail -n1)
PS_BODY=$(echo "$PS_RESPONSE" | sed '$d')

if [ "$PS_HTTP" = "201" ]; then
	PS_IDENTITY_ID=$(echo "$PS_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
	echo -e "${GREEN}✓ 'Identity Access' permission set created (ID: $PS_IDENTITY_ID)${NC}"
elif [ "$PS_HTTP" = "409" ]; then
	PS_IDENTITY_ID=$(lookup_id_by_field "permission-sets" "name" "Identity Access")
	echo -e "${YELLOW}✓ 'Identity Access' permission set already exists (ID: $PS_IDENTITY_ID)${NC}"
else
	echo -e "${RED}✗ Failed to create 'Identity Access' permission set (HTTP $PS_HTTP)${NC}"
	echo "  Response: $PS_BODY"
	echo "  Note: permission-sets endpoint may not be implemented yet"
	PS_IDENTITY_ID=""
fi

# PS2: "Weather Access" (optional) — covers weather.read + weather.forecast from Weather API
# ServiceScope requirement_type is mandatory: when this PS is selected, Weather API is always included.
echo "Creating 'Weather Access' permission set (optional, service scope: mandatory)..."
PS_RESPONSE=$(curl -s -X POST "${ADMIN_API}/permission-sets" \
	-H "Content-Type: application/json" \
	-w "\n%{http_code}" \
	-d "{
    \"name\": \"Weather Access\",
    \"description\": \"Read current weather conditions and forecasts\",
    \"service_scopes\": [
      {
        \"service_id\": \"${WEATHER_SERVICE_ID}\",
        \"scopes\": [\"weather.read\", \"weather.forecast\"],
        \"requirement_type\": \"mandatory\"
      }
    ]
  }")

PS_HTTP=$(echo "$PS_RESPONSE" | tail -n1)
PS_BODY=$(echo "$PS_RESPONSE" | sed '$d')

if [ "$PS_HTTP" = "201" ]; then
	PS_WEATHER_ID=$(echo "$PS_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
	echo -e "${GREEN}✓ 'Weather Access' permission set created (ID: $PS_WEATHER_ID)${NC}"
elif [ "$PS_HTTP" = "409" ]; then
	PS_WEATHER_ID=$(lookup_id_by_field "permission-sets" "name" "Weather Access")
	echo -e "${YELLOW}✓ 'Weather Access' permission set already exists (ID: $PS_WEATHER_ID)${NC}"
else
	echo -e "${RED}✗ Failed to create 'Weather Access' permission set (HTTP $PS_HTTP)${NC}"
	echo "  Response: $PS_BODY"
	PS_WEATHER_ID=""
fi

# PS3: "Calendar Sync" (optional) — covers calendar.read + calendar.write from Calendar API
echo "Creating 'Calendar Sync' permission set (optional)..."
PS_RESPONSE=$(curl -s -X POST "${ADMIN_API}/permission-sets" \
	-H "Content-Type: application/json" \
	-w "\n%{http_code}" \
	-d "{
    \"name\": \"Calendar Sync\",
    \"description\": \"Read and write calendar events for scheduling assistance\",
    \"service_scopes\": [
      {
        \"service_id\": \"${CALENDAR_SERVICE_ID}\",
        \"scopes\": [\"calendar.read\", \"calendar.write\"]
      }
    ]
  }")

PS_HTTP=$(echo "$PS_RESPONSE" | tail -n1)
PS_BODY=$(echo "$PS_RESPONSE" | sed '$d')

if [ "$PS_HTTP" = "201" ]; then
	PS_CALENDAR_ID=$(echo "$PS_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
	echo -e "${GREEN}✓ 'Calendar Sync' permission set created (ID: $PS_CALENDAR_ID)${NC}"
elif [ "$PS_HTTP" = "409" ]; then
	PS_CALENDAR_ID=$(lookup_id_by_field "permission-sets" "name" "Calendar Sync")
	echo -e "${YELLOW}✓ 'Calendar Sync' permission set already exists (ID: $PS_CALENDAR_ID)${NC}"
else
	echo -e "${RED}✗ Failed to create 'Calendar Sync' permission set (HTTP $PS_HTTP)${NC}"
	echo "  Response: $PS_BODY"
	PS_CALENDAR_ID=""
fi
echo ""

# ==========================================
# Step 4: Create Simple Agents
# FR-006: all agents require at least one permission set declaration. Simple agents
# here have no service requirements; they reference "Identity Access" as optional so
# users can optionally connect an identity provider without being forced to.
# ==========================================
echo "========================================="
echo "Step 4: Creating Simple Agents"
echo "========================================="
echo ""

if [ -z "$PS_IDENTITY_ID" ]; then
	echo -e "${YELLOW}Skipping simple agents — 'Identity Access' PS not available (endpoint may not be implemented yet)${NC}"
	echo ""
else

	echo "Creating Weather Assistant agent..."
	AGENT_RESPONSE=$(curl -s -X POST "${ADMIN_API}/agents" \
		-H "Content-Type: application/json" \
		-w "\n%{http_code}" \
		-d "{
    \"client_id\": \"weather-assistant-001\",
    \"display_name\": \"Weather Assistant\",
    \"description\": \"AI assistant that helps you check weather forecasts and conditions\",
    \"governance_url\": \"https://example.com/weather-assistant/governance\",
    \"user_documentation_url\": \"https://example.com/weather-assistant/docs\",
    \"agent_interface_url\": \"https://example.com/weather-assistant\",
    \"service_requirements\": [
      {
        \"service_id\": \"${MOCK_SERVICE_ID}\",
        \"requirement_type\": \"optional\",
        \"required_scopes\": [\"profile\", \"email\"]
      }
    ],
    \"permission_sets\": [
      {\"permission_set_id\": \"${PS_IDENTITY_ID}\", \"requirement_type\": \"optional\"}
    ]
  }")

	HTTP_CODE=$(echo "$AGENT_RESPONSE" | tail -n1)
	if [ "$HTTP_CODE" = "201" ]; then
		echo -e "${GREEN}✓ Weather Assistant created${NC}"
	elif [ "$HTTP_CODE" = "409" ]; then
		echo -e "${YELLOW}✓ Weather Assistant already exists${NC}"
	else
		echo -e "${RED}✗ Failed to create Weather Assistant (HTTP $HTTP_CODE)${NC}"
		exit 1
	fi
	echo ""

	echo "Creating Task Manager agent..."
	AGENT_RESPONSE=$(curl -s -X POST "${ADMIN_API}/agents" \
		-H "Content-Type: application/json" \
		-w "\n%{http_code}" \
		-d "{
    \"client_id\": \"task-manager-agent\",
    \"display_name\": \"Task Manager Pro\",
    \"description\": \"Intelligent task management assistant that helps organize your work\",
    \"governance_url\": \"https://example.com/task-manager/governance\",
    \"user_documentation_url\": \"https://example.com/task-manager/docs\",
    \"service_requirements\": [
      {
        \"service_id\": \"${MOCK_SERVICE_ID}\",
        \"requirement_type\": \"optional\",
        \"required_scopes\": [\"profile\", \"email\"]
      }
    ],
    \"permission_sets\": [
      {\"permission_set_id\": \"${PS_IDENTITY_ID}\", \"requirement_type\": \"optional\"}
    ]
  }")

	HTTP_CODE=$(echo "$AGENT_RESPONSE" | tail -n1)
	if [ "$HTTP_CODE" = "201" ]; then
		echo -e "${GREEN}✓ Task Manager Pro created${NC}"
	elif [ "$HTTP_CODE" = "409" ]; then
		echo -e "${YELLOW}✓ Task Manager Pro already exists${NC}"
	else
		echo -e "${RED}✗ Failed to create Task Manager Pro (HTTP $HTTP_CODE)${NC}"
		exit 1
	fi
	echo ""

fi # end PS_IDENTITY_ID guard

# ==========================================
# Step 5: Create Permission-Set Test Agent
# This agent has:
#   - Mock OAuth2 Service: MANDATORY service requirement
#   - Weather API:         OPTIONAL service requirement  (toggle-off hides it)
#   - Calendar API:        OPTIONAL service requirement  (toggle-off hides it)
#
# Permission sets (only created when the PS endpoint is available):
#   - "Identity Access" PS: mandatory — always shown, user cannot opt out
#   - "Weather Access" PS:  optional  — user can toggle off to exclude Weather API
#   - "Calendar Sync" PS:   optional  — user can toggle off to exclude Calendar API
# ==========================================
echo "========================================="
echo "Step 5: Creating Permission-Set Test Agent"
echo "========================================="
echo ""

if [ -n "$PS_IDENTITY_ID" ] && [ -n "$PS_WEATHER_ID" ] && [ -n "$PS_CALENDAR_ID" ]; then
	# Full PS-based agent — all three permission sets available
	AGENT_BODY="{
    \"client_id\": \"upstream-oauth2-client\",
    \"display_name\": \"Sample Agent\",
    \"description\": \"Demo agent with one mandatory and two optional service requirements for permission-set toggle testing\",
    \"governance_url\": \"https://example.com/sample-agent/governance\",
    \"user_documentation_url\": \"https://example.com/sample-agent/docs\",
    \"agent_interface_url\": \"http://localhost:8000\",
    \"redirect_uris\": [\"http://localhost:9002/oauth2/callback\"],
    \"service_requirements\": [
      {
        \"service_id\": \"${MOCK_SERVICE_ID}\",
        \"requirement_type\": \"mandatory\",
        \"required_scopes\": [\"profile\", \"email\"]
      },
      {
        \"service_id\": \"${WEATHER_SERVICE_ID}\",
        \"requirement_type\": \"optional\",
        \"required_scopes\": [\"weather.read\", \"weather.forecast\"]
      },
      {
        \"service_id\": \"${CALENDAR_SERVICE_ID}\",
        \"requirement_type\": \"optional\",
        \"required_scopes\": [\"calendar.read\", \"calendar.write\"]
      }
    ],
    \"permission_sets\": [
      {
        \"permission_set_id\": \"${PS_IDENTITY_ID}\",
        \"requirement_type\": \"mandatory\"
      },
      {
        \"permission_set_id\": \"${PS_WEATHER_ID}\",
        \"requirement_type\": \"optional\"
      },
      {
        \"permission_set_id\": \"${PS_CALENDAR_ID}\",
        \"requirement_type\": \"optional\"
      }
    ]
  }"
	echo "Creating Sample Agent with permission sets and optional service requirements..."
else
	# Fallback: no permission sets yet (endpoint not implemented)
	AGENT_BODY="{
    \"client_id\": \"upstream-oauth2-client\",
    \"display_name\": \"Sample Agent\",
    \"description\": \"Demo agent (permission-sets endpoint not yet available)\",
    \"governance_url\": \"https://example.com/sample-agent/governance\",
    \"user_documentation_url\": \"https://example.com/sample-agent/docs\",
    \"agent_interface_url\": \"http://localhost:8000\"
  }"
	echo -e "${YELLOW}Creating Sample Agent without permission sets (PS endpoint not available)${NC}"
fi

AGENT_RESPONSE=$(curl -s -X POST "${ADMIN_API}/agents" \
	-H "Content-Type: application/json" \
	-w "\n%{http_code}" \
	-d "$AGENT_BODY")

HTTP_CODE=$(echo "$AGENT_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$AGENT_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
	AGENT_ID=$(echo "$RESPONSE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
	echo -e "${GREEN}✓ Sample Agent created (ID: $AGENT_ID)${NC}"
	if [ -n "$PS_IDENTITY_ID" ]; then
		echo "  Service requirements:"
		echo "    - Mock OAuth2 Service (mandatory)"
		echo "    - Weather API         (optional — 'Weather Access' PS toggles this)"
		echo "    - Calendar API        (optional — 'Calendar Sync' PS toggles this)"
		echo "  Permission sets:"
		echo "    - Identity Access (mandatory)"
		echo "    - Weather Access  (optional)"
		echo "    - Calendar Sync   (optional)"
	fi
elif [ "$HTTP_CODE" = "409" ]; then
	echo -e "${YELLOW}✓ Sample Agent already exists${NC}"
	AGENT_IDS=$(curl -s "${ADMIN_API}/agents" -H "X-Remote-User: admin@example.com" | grep -o '"id":"[^"]*","client_id":"upstream-oauth2-client"' | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
	AGENT_ID=$(printf '%s\n' "$AGENT_IDS" | sed -n '1p')
	SECOND_ID=$(printf '%s\n' "$AGENT_IDS" | sed -n '2p')
	if [ -n "$SECOND_ID" ]; then
		echo -e "${RED}✗ Multiple agents found for client_id=upstream-oauth2-client; cannot resolve unambiguously${NC}"
		exit 1
	fi
	echo "  Agent ID: $AGENT_ID"
else
	echo -e "${RED}✗ Failed to create Sample Agent${NC}"
	echo "  HTTP Status: $HTTP_CODE"
	echo "  Response: $RESPONSE_BODY"
	exit 1
fi
echo ""

# ==========================================
# Step 6: Create Local and CIMD Agents
# Local agent: no client_id, no client_uris -> broker issues tokens locally.
# CIMD agent:  no client_id, client_uris set -> broker resolves via CIMD fetch.
#   client_uris points to the cimd-mock container (https://cimd-mock/oauth/client-metadata.json).
# ==========================================
echo "========================================="
echo "Step 6: Creating Local and CIMD Agents"
echo "========================================="
echo ""

echo "Creating Local Research Agent (no client_id — local token issuance)..."
if [ -n "$PS_IDENTITY_ID" ]; then
	LOCAL_PS_FIELD=", \"permission_sets\": [{\"permission_set_id\": \"${PS_IDENTITY_ID}\", \"requirement_type\": \"optional\"}]"
else
	LOCAL_PS_FIELD=""
fi
if [ -n "$MOCK_SERVICE_ID" ]; then
	LOCAL_SR_FIELD=", \"service_requirements\": [{\"service_id\": \"${MOCK_SERVICE_ID}\", \"requirement_type\": \"optional\", \"required_scopes\": [\"profile\", \"email\"]}]"
else
	LOCAL_SR_FIELD=""
fi
AGENT_RESPONSE=$(curl -s -X POST "${ADMIN_API}/agents" \
	-H "Content-Type: application/json" \
	-w "\n%{http_code}" \
	-d "{
    \"display_name\": \"Local Research Agent\",
    \"description\": \"Agent issued local tokens (no upstream client_id)\",
    \"governance_url\": \"https://example.com/local-agent/governance\",
    \"user_documentation_url\": \"https://example.com/local-agent/docs\",
    \"redirect_uris\": [\"http://localhost:9002/oauth2/callback\"]${LOCAL_SR_FIELD}${LOCAL_PS_FIELD}
  }")

HTTP_CODE=$(echo "$AGENT_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$AGENT_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
	LOCAL_AGENT_ID=$(echo "$RESPONSE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
	echo -e "${GREEN}✓ Local Research Agent created (ID: $LOCAL_AGENT_ID)${NC}"
elif [ "$HTTP_CODE" = "409" ]; then
	echo -e "${YELLOW}✓ Local Research Agent already exists${NC}"
else
	echo -e "${RED}✗ Failed to create Local Research Agent (HTTP $HTTP_CODE)${NC}"
	echo "  Response: $RESPONSE_BODY"
	exit 1
fi
echo ""

echo "Creating CIMD Demo Agent (placeholder client_uri — fetch will fail until mock server is available)..."
if [ -n "$PS_IDENTITY_ID" ]; then
	CIMD_PS_FIELD=", \"permission_sets\": [{\"permission_set_id\": \"${PS_IDENTITY_ID}\", \"requirement_type\": \"optional\"}]"
else
	CIMD_PS_FIELD=""
fi
if [ -n "$MOCK_SERVICE_ID" ]; then
	CIMD_SR_FIELD=", \"service_requirements\": [{\"service_id\": \"${MOCK_SERVICE_ID}\", \"requirement_type\": \"optional\", \"required_scopes\": [\"profile\", \"email\"]}]"
else
	CIMD_SR_FIELD=""
fi
AGENT_RESPONSE=$(curl -s -X POST "${ADMIN_API}/agents" \
	-H "Content-Type: application/json" \
	-w "\n%{http_code}" \
	-d "{
    \"display_name\": \"CIMD Demo Agent\",
    \"description\": \"Agent resolved via Client ID Metadata Document (placeholder URL)\",
    \"governance_url\": \"https://example.com/cimd-agent/governance\",
    \"user_documentation_url\": \"https://example.com/cimd-agent/docs\",
    \"redirect_uris\": [\"http://localhost:9002/oauth2/callback\"],
    \"client_uris\": [\"https://${CIMD_SERVER_HOST}/oauth/client-metadata.json\"]${CIMD_SR_FIELD}${CIMD_PS_FIELD}
  }")

HTTP_CODE=$(echo "$AGENT_RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$AGENT_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
	CIMD_AGENT_ID=$(echo "$RESPONSE_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
	echo -e "${GREEN}✓ CIMD Demo Agent created (ID: $CIMD_AGENT_ID)${NC}"
elif [ "$HTTP_CODE" = "409" ]; then
	echo -e "${YELLOW}✓ CIMD Demo Agent already exists${NC}"
else
	echo -e "${RED}✗ Failed to create CIMD Demo Agent (HTTP $HTTP_CODE)${NC}"
	echo "  Response: $RESPONSE_BODY"
	exit 1
fi
echo ""

echo "========================================="
echo "Development Environment Setup Complete"
echo "========================================="
echo ""
echo "Summary:"
echo "  Agents:"
echo "    - Weather Assistant      (no service requirements)"
echo "    - Task Manager Pro       (no service requirements)"
echo "    - Sample Agent           (upstream-oauth2-client, proxy path)"
if [ -n "$PS_IDENTITY_ID" ]; then
	echo "        SR: Mock OAuth2 (mandatory), Weather API (optional), Calendar API (optional)"
	echo "        PS: Identity Access (mandatory), Weather Access (optional), Calendar Sync (optional)"
else
	echo "        (permission sets skipped — endpoint not available)"
fi
echo "    - Local Research Agent   (no client_id, local token issuance)"
echo "    - CIMD Demo Agent        (client_uris: https://${CIMD_SERVER_HOST}/oauth/client-metadata.json)"
echo ""
echo "  Services:"
echo "    - Mock OAuth2 Service    (mock-oauth2-client-dev)"
echo "    - Weather API            (weather-api-client)"
echo "    - Calendar API           (calendar-api-client)"
echo "    - Email API              (email-api-client)"
echo ""
if [ -n "$PS_IDENTITY_ID" ]; then
	echo "  Permission Sets:"
	echo "    - Identity Access        (mandatory) — profile + email from Mock OAuth2"
	echo "    - Weather Access         (optional)  — weather.read + weather.forecast (service SR: mandatory)"
	echo "    - Calendar Sync          (optional)  — calendar.read + calendar.write"
	echo ""
	echo "Testing optional service toggle:"
	echo "  1. Open consent UI at http://localhost:8000"
	echo "  2. Navigate to Sample Agent consent screen"
	echo "  3. 'Weather Access' PS card:  toggle OFF to exclude Weather API"
	echo "  4. 'Calendar Sync' PS card:   toggle OFF to exclude Calendar API"
	echo "  5. When both optional PSs are off, only Mock OAuth2 connection is required"
fi
echo ""
echo "Access services at:"
echo "  - Frontend and API:      http://localhost:8000"
echo "  - Admin API:             http://localhost:14000"
echo ""
