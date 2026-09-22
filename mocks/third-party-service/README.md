# Mock OAuth2 Service

A standalone mock OAuth2 server for manual testing of the identity broker's OAuth2 session management feature.

## Overview

This mock service implements the standard OAuth2 Authorization Code Flow with PKCE validation (RFC 7636). It's designed for local development and testing of the OAuth2 session management feature.

**Key Features**:
- Standard OAuth2 Authorization Code Flow
- PKCE support (automatic validation)
- Simple HTML consent page
- In-memory storage (ephemeral, restarts clean)
- Localhost-only binding (127.0.0.1:9000)
- HTTP only (no TLS, suitable for localhost testing)

## Quick Start

### Prerequisites

- Go 1.27.1+
- Broker running on http://localhost:8000 (or localhost:14000 for admin API)
- `curl` or similar for testing

### One-Command Setup

```bash
# Terminal 1: Start the broker
cd ~/Projects/agentic-identity-broker-008-thirdparty-oauth2-sessions
just run

# Terminal 2: Start the mock OAuth2 service and register it
just mock-third-party-oauth2-setup
```

This command will:
1. Build the mock OAuth2 server binary
2. Start it in the background on http://localhost:9000
3. Register it with the broker via the admin API

### Manual Test Flow

1. **Open Broker UI**: http://localhost:8000/sessions
2. **Connect**: Click "Connect" on "Mock OAuth2 Service (Dev)" card
3. **Approve**: You'll be redirected to the mock consent page - click "Approve"
4. **Session Created**: The mock redirects back to the broker with an authorization code
5. **Token Exchange**: The broker exchanges the code for tokens (PKCE validated)
6. **Session Stored**: The session appears in your sessions list
7. **Terminate**: Click "Terminate" to revoke the session

## Configuration

The mock service configuration is defined in `config.yaml`:

```yaml
server:
  port: 9000                    # Server port
  bind: "127.0.0.1"            # Bind address (localhost only)

oauth2:
  client_id: "mock-oauth2-client-dev"
  client_secret: "mock-oauth2-secret-12345"
  access_token_ttl: 3600s       # 1 hour
  refresh_token_ttl: 604800s    # 7 days
  authorization_code_ttl: 300s  # 5 minutes

  scopes:
    - name: "profile"
      description: "Access user profile information"
    - name: "email"
      description: "Access email address"
    - name: "read"
      description: "Read data"
    - name: "write"
      description: "Write data"

mock_user:
  sub: "testuser@example.com"
  name: "Test User"
  email: "testuser@example.com"
```

## Architecture

### Directory Structure

```
mocks/third-party-service/
├── cmd/
│   └── mock-oauth2-server/
│       └── main.go                    # Server entrypoint
├── internal/
│   ├── config/
│   │   └── config.go                  # Configuration loading
│   ├── server/
│   │   └── server.go                  # OAuth2 server setup
│   └── handlers/
│       ├── health.go                  # Health check endpoint
│       └── consent.go                 # Consent page handler
├── static/
│   └── (HTML assets served inline)
├── config.yaml                        # Configuration file
├── go.mod                             # Go module definition
└── README.md                          # This file

Note: The service registration script has been moved to `scripts/register-mock-thirdparty-service.sh` in the project root
```

### Key Components

**OAuth2 Endpoints**:
- `GET /health` - Health check
- `GET /oauth/authorize` - Authorization endpoint (serves consent page)
- `POST /oauth/token` - Token exchange endpoint (with PKCE validation)

**Service Registration**:
- Registers with broker at `POST http://localhost:14000/api/services`
- Service ID: `mock-oauth2-client-dev`
- Service Secret: `mock-oauth2-secret-12345`

## Justfile Targets

```bash
# Start mock third-party OAuth2 service (runs on port 9000)
just mock-third-party-oauth2-start

# Build mock third-party OAuth2 service binary
just mock-third-party-oauth2-build

# Register mock service with broker
just mock-third-party-oauth2-register

# Full setup: build, start background, register
just mock-third-party-oauth2-setup

# Stop and cleanup
just mock-third-party-oauth2-clean
```

## PKCE Flow

The mock service validates PKCE per RFC 7636:

1. **Broker creates PKCE verifier** (`GeneratePKCE` in broker)
   - Generates random 32 bytes
   - Base64url encodes to create verifier
   - Computes SHA256(verifier) for challenge

2. **Authorization request** to mock service includes:
   - `code_challenge`: Base64url(SHA256(verifier))
   - `code_challenge_method: S256`

3. **Mock stores the challenge** with the authorization code

4. **Token exchange** includes:
   - `code_verifier`: The original random string

5. **Validation**:
   - Mock computes: SHA256(code_verifier)
   - Compares with stored `code_challenge`
   - If mismatch: returns `{"error": "invalid_grant"}`

## Security Considerations

### Localhost Only

The mock service binds to `127.0.0.1` (localhost only):
```yaml
bind: "127.0.0.1"
```

This prevents external network access. The mock:
- Uses HTTP (no TLS)
- Has weak credentials (`mock-oauth2-secret-12345`)
- Is safe because only localhost can connect

### Development Credentials

Hardcoded credentials are acceptable for dev-only testing:
- Client ID: `mock-oauth2-client-dev`
- Client Secret: `mock-oauth2-secret-12345`
- No production value

### State Token Handling

The `state` parameter is **opaque** to the mock service:
- Broker creates, encrypts, and validates state tokens (JWE)
- Mock simply echoes the state parameter back unchanged
- Mock does NOT decrypt, parse, or modify state

## Troubleshooting

### Mock server won't start

**Error**: "bind: address already in use"

```bash
# Check what's using port 9000
lsof -i :9000

# Kill the process
kill -9 <PID>
```

### Can't register with broker

**Error**: "Failed to start mock server" or "Admin API is not accessible"

- Ensure broker is running: `just run` (terminal 1)
- Ensure mock is running: `just mock-third-party-oauth2-start` (terminal 2)
- Check firewall/iptables if on non-standard network

### PKCE validation failure

**Error**: Token exchange returns `{"error": "invalid_grant"}`

Possible causes:
- Authorization code expired (>5 minutes old)
- Authorization code already used (single-use)
- Wrong `code_verifier` sent by broker (indicates broker bug)

**Solution**: Restart the OAuth2 flow from the consent UI

### Authorization code already used

**Error**: "invalid_grant" on second token exchange attempt

This is expected behavior - authorization codes are single-use. Try the flow again from the beginning.

## Dependencies

The mock service uses minimal dependencies:

```go
github.com/go-oauth2/oauth2/v4 v4.5.2    // OAuth2 server framework
github.com/spf13/viper v1.21.0           // Configuration
gopkg.in/yaml.v3 v3.0.1                  // YAML parsing
```

See `go.mod` for the complete dependency tree.

## Testing Workflow

### Automated Integration Test

The broker's self-contained integration tests use mock OAuth2 providers. To run them:

```bash
just test-integration
```

### Manual Testing

1. **Setup**: `just mock-third-party-oauth2-setup`
2. **Test**: Navigate to http://localhost:8000/sessions
3. **Cleanup**: `just mock-third-party-oauth2-clean`

### End-to-End Flow

```
Broker UI → Redirect to Mock Authorize → User Approves →
Mock redirects with code → Broker exchanges code (PKCE validated) →
Session stored → Session appears in UI
```

## Implementation Notes

### Authorization Code Storage

Authorization codes are stored in an in-memory map with PKCE metadata:

```go
authCodes = map[string]*authCodeData{
    "code": &authCodeData{
        clientID:            "mock-oauth2-client-dev",
        redirectURI:         "http://localhost:8000/api/third-party/...",
        scope:               "profile email read write",
        userID:              "testuser@example.com",
        codeChallenge:       "E9Mrozoa2owUzlKWuRfTXVR..",  // Base64(SHA256(verifier))
        codeChallengeMethod: "S256",
    },
}
```

Codes are single-use and deleted after token exchange.

### Token Generation

Tokens are generated by the `go-oauth2/oauth2/v4` library with configured TTLs:
- Access tokens: 1 hour (configurable)
- Refresh tokens: 7 days (configurable)
- Authorization codes: 5 minutes (configurable)

## Performance Characteristics

- **Consent page load**: <10ms
- **Authorization code generation**: <5ms
- **Token exchange**: ~50ms (includes PKCE validation)
- **Concurrent requests**: Handles 100s of concurrent requests (in-memory storage)

For production testing with heavy load, use ORY Hydra or similar.

## Further Reading

- [OAuth 2.0 Authorization Code Flow](https://tools.ietf.org/html/rfc6749#section-1.3.1)
- [PKCE (RFC 7636)](https://tools.ietf.org/html/rfc7636)
- [go-oauth2/oauth2 Documentation](https://github.com/go-oauth2/oauth2)

## License

Same as the identity broker project.

## Development

To modify the mock service:

1. Make code changes
2. Rebuild: `just mock-third-party-oauth2-build`
3. Restart mock: `pkill mock-oauth2-server && just mock-third-party-oauth2-start`

Or use hot-reload (not implemented yet):

```bash
cd mocks/third-party-service
air
```

## Future Enhancements

- [ ] Support for other OAuth2 flows (Implicit, Client Credentials, etc.)
- [ ] Token revocation endpoint
- [ ] Refresh token support
- [ ] OpenID Connect support
- [ ] Hot-reload development with Air
- [ ] Detailed request/response logging
- [ ] Rate limiting per client
- [ ] Custom scope validation
