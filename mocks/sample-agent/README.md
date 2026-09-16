# Sample OAuth2 Client Agent

This is a sample OAuth2 client application that demonstrates the complete end-to-end OAuth2 authorization flow using the identity broker as the authorization server.

## Overview

The Sample Agent showcases a realistic OAuth2 client that:
- Initiates OAuth2 authorization flow
- Redirects users to the identity broker for authentication and consent
- Receives authorization code and exchanges it for access token
- Displays user information after successful authentication
- Manages user sessions
- Calls the Compose MCP mock through Agentgateway, including a Rego-gated `create_issue` demo
  that opens a tool-approval request

## Architecture

```
Sample Agent (OAuth2 Client)
    ↓ (Redirects for authentication)
Identity Broker (OAuth2 Authorization Server)
    ↓ (Proxies to upstream)
Upstream OAuth2 Mock Server (Real Authorization Server)
    ↓ (Returns authorization code)
Back to Sample Agent (Callback)
    ↓ (Token exchange)
Token Received → User Info Displayed
```

## Configuration

The sample agent uses the upstream OAuth2 mock server credentials for testing the complete flow:

```yaml
server:
  port: 8001                          # Sample agent port
  bind: "127.0.0.1"

oauth2:
  client_id: "upstream-oauth2-client"
  client_secret: "upstream-oauth2-secret-xyz"
  redirect_uri: "http://127.0.0.1:8001/oauth2/callback"
  scopes: [openid, profile, email]

broker:
  base_url: "http://localhost:8000"
  authorize_endpoint: "http://localhost:8000/oauth2/authorize"
  token_endpoint: "http://localhost:8000/oauth2/token"
```

## Running the Sample Agent

### From Project Root

```bash
# Build and run
go run ./mocks/sample-agent/cmd/sample-agent/main.go

# Or use justfile target
just mock-sample-agent-start
```

### From the Agent Directory

```bash
cd mocks/sample-agent
go run ./cmd/sample-agent/main.go
```

The agent will start on `http://127.0.0.1:8001`.

## Testing the Complete Flow

### Prerequisites

You need three servers running:

1. **Upstream OAuth2 Mock Server** (port 9001)
   ```bash
   just mock-upstream-oauth2-start
   ```

2. **Identity Broker** (port 8000)
   ```bash
   just dev
   ```

3. **Sample OAuth2 Client** (port 8001)
   ```bash
   just mock-sample-agent-start
   ```

### Complete Flow Test

1. **Start all three services** (in separate terminals):
   - Terminal 1: `just mock-upstream-oauth2-start`
   - Terminal 2: `just dev`
   - Terminal 3: `just mock-sample-agent-start`

2. **Open browser**: `http://127.0.0.1:8001`

3. **Click "Login with OAuth2"**
   - You'll see the distinctive identity broker login page
   - (Note: In development, authentication is simulated via X-Remote-User header)

4. **Consent Flow**
   - You'll be redirected to the upstream OAuth2 server (port 9001)
   - Notice the **DISTINCTIVE TEAL BACKGROUND** with 🌐 UPSTREAM OAUTH2 badge
   - This proves you're going through the complete proxy flow

5. **Approval**
   - Click "✓ Approve" on the consent screen
   - You'll be redirected back with an authorization code

6. **Token Exchange**
   - The sample agent exchanges the code for an access token
   - User information is displayed

7. **User Information Page**
   - Shows subject, email, token expiry, session ID
   - Confirms successful OAuth2 flow completion

### Tool Approval Demonstration

With `just compose-up` running, sign in as the Proxy Client and complete the delegation flow. The
authenticated page includes **Call MCP Tool: create_issue (approval required)**. The bundled OPA
policy returns `approval_required` for that tool with stable `repository` and `title` arguments.
Use the resulting review link, select **For this session**, and expand **Approval scope**. Change
each parameter from **This value** (exact) to **Any value** or **Custom match**. The broker
validates custom globs such as `acme/*` before you approve. The Sample Agent retains one MCP
client for its web session, so the next button click uses the same MCP session and forwards.

## API Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/health` | GET | Health check |
| `/` | GET | Home page (login or user info) |
| `/login` | GET | Initiate OAuth2 authorization flow |
| `/oauth2/callback` | GET | OAuth2 callback endpoint (receives code) |
| `/logout` | GET | Clear session and logout |

## HTTP Flow

### 1. Initial Request
```
GET / → Shows login page
```

### 2. User Clicks Login
```
GET /login → Redirect to Identity Broker's authorize endpoint
→ Sample Agent → Identity Broker (port 8000)
```

### 3. Identity Broker Authorization
```
GET /oauth2/authorize (at broker)
→ Validates client_id against agents
→ Checks consent status
→ No grant → Redirect to consent UI (or use existing grant)
```

### 4. Upstream OAuth2 Server
```
If proxying to upstream (port 9001):
GET /oauth/authorize → Teal consent page with 🌐 UPSTREAM OAUTH2
User approves → Returns authorization code
```

### 5. Callback to Sample Agent
```
GET /oauth2/callback?code=<auth_code>&state=<state>
Sample Agent receives code
```

### 6. Token Exchange
```
POST /oauth2/token (at broker)
Sample Agent → Identity Broker
Identity Broker proxies to Upstream → http://127.0.0.1:9001/oauth/token
Receives access_token in response
```

### 7. User Info Display
```
Token stored in session
User information displayed
```

## Session Management

- Sessions are stored in-memory with HTTP cookies
- Session ID is stored in `session_id` cookie (HttpOnly, SameSite=Lax)
- Session includes OAuth2 token and user info
- Sessions expire after 24 hours
- Logout clears the session

## Visual Indicators

### Login Page
- Purple gradient background
- Clear explanation of the OAuth2 flow
- Shows the three-tier architecture

### Upstream Consent (when reaching port 9001)
- **DISTINCTIVE TEAL BACKGROUND** (#00d4aa)
- 🌐 UPSTREAM OAUTH2 badge
- Proves successful proxy to upstream server

### User Info Page
- Shows all OAuth2-provided information
- Displays token expiry time
- Shows session ID for debugging
- Confirms successful end-to-end flow

## Debugging

### Health Check
```bash
curl http://127.0.0.1:8001/health
# {"status":"ok","service":"sample-oauth2-client"}
```

### Check Logs
Monitor the logs to see:
- Authorization flow initiation
- Code exchange attempts
- Token receipt
- Session creation

### Common Issues

**Port 8001 already in use?**
```bash
# Edit config.yaml and change port, or:
lsof -i :8001
kill -9 <PID>
```

**Callback not working?**
- Ensure redirect_uri matches: `http://127.0.0.1:8001/oauth2/callback`
- Check that identity broker is running on port 8000
- Verify upstream server is running on port 9001

**Token exchange fails?**
- Check broker logs for proxy errors
- Verify upstream server is responsive
- Ensure client_id and secret match upstream mock

## Files

| File | Purpose |
|------|---------|
| `cmd/sample-agent/main.go` | Entry point |
| `internal/config/config.go` | Configuration loading |
| `internal/server/server.go` | HTTP server setup |
| `internal/handlers/handlers.go` | HTTP handlers and OAuth2 logic |
| `go.mod` | Go module definition |
| `config.yaml` | Configuration file |
| `README.md` | This documentation |
| `.gitignore` | Git ignore rules |

## Dependencies

- `golang.org/x/oauth2` - OAuth2 client library (standard Go OAuth2 library)
- `gopkg.in/yaml.v3` - YAML configuration parsing

## Justfile Targets

```bash
just mock-sample-agent-start       # Start the sample agent (port 8001)
just mock-sample-agent-build       # Build the binary
just mock-sample-agent-test        # Run tests
just mock-sample-agent-setup       # Build + start in background + health check
just mock-sample-agent-clean       # Stop and clean up
```

## End-to-End Testing Workflow

```bash
# Terminal 1: Start upstream mock (port 9001)
just mock-upstream-oauth2-start

# Terminal 2: Start identity broker (port 8000)
just dev

# Terminal 3: Start sample agent (port 8001)
just mock-sample-agent-start

# Browser: Visit http://127.0.0.1:8001
# 1. Click "Login with OAuth2"
# 2. You'll see identity broker page
# 3. When redirected to port 9001, you'll see TEAL background
# 4. Approve the consent
# 5. You'll see user information page showing successful OAuth2 flow
```

## Key Test Points

✅ Verify sample agent starts on port 8001
✅ Verify login page displays correctly
✅ Verify redirect to identity broker (port 8000)
✅ Verify redirect to upstream server shows TEAL background
✅ Verify consent and approval flow
✅ Verify token exchange succeeds
✅ Verify user information is displayed
✅ Verify logout clears session
✅ Verify complete end-to-end flow works

## Next Steps

1. Start all three services
2. Test the complete OAuth2 flow
3. Verify the distinctive teal consent page appears
4. Confirm token exchange succeeds
5. View the user information page
6. Logout and repeat

This demonstrates a realistic OAuth2 client integration with the identity broker proxy architecture!
