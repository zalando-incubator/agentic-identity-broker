# ExtProc gRPC Contract: Token Exchange

**Protocol**: Envoy External Processing (ExtProc) gRPC v3  
**Proto**: `envoy.service.ext_proc.v3.ExternalProcessor`
> **Implementation note (superseded raw input):**
> [Feature 043](../../043-extproc-metadata-input/contracts/extproc-metadata-input.md) and [ADR 036](../../../adrs/036-extproc-metadata-token-exchange-input.md) replace raw `Authorization` and pseudo-header input, no-Bearer pass-through, and raw resource validation.
> This document retains the standalone process, configuration, cache, circuit-breaker, and exchange mechanics that remain applicable.


## Service Definition

The ExtProc service implements the standard Envoy External Processor gRPC interface. It does **not** define its own API — it integrates with the Envoy/agentgateway ExtProc protocol.

### gRPC Service

```protobuf
// From envoy.service.ext_proc.v3
service ExternalProcessor {
  rpc Process(stream ProcessingRequest) returns (stream ProcessingResponse);
}
```

### Health Check Service

```protobuf
// Standard gRPC health check
service Health {
  rpc Check(HealthCheckRequest) returns (HealthCheckResponse);
  rpc Watch(HealthCheckRequest) returns (stream HealthCheckResponse);
}
```

## Request Processing Behavior

### RequestHeaders Phase

This is the **primary processing phase** where token exchange occurs.

#### Input: ProcessingRequest_RequestHeaders

The current input contract is [Feature 043](../../043-extproc-metadata-input/contracts/extproc-metadata-input.md). ExtProc reads `subject_token` and `resource_uri` only from `aib.tokenexchange` dynamic metadata. Raw HTTP attributes remain available only for transport and OPA checks.


#### Output Scenarios

**Scenario 1: Missing or Invalid Metadata → Immediate 503 Response**

Feature 043 defines the fixed `invalid_subject_token` and `invalid_resource` JSON responses. Raw `Authorization` and pseudo-headers cannot supply token-exchange input.


**Scenario 2: Valid Bearer Token → Replace Authorization**

When token exchange succeeds:

```
ProcessingResponse{
  Response: ProcessingResponse_RequestHeaders{
    RequestHeaders: HeadersResponse{
      Response: CommonResponse{
        HeaderMutation: HeaderMutation{
          SetHeaders: [
            HeaderValueOption{
              Header: HeaderValue{
                Key: "authorization",
                RawValue: "Bearer <exchanged_token>"
              }
            }
          ]
        }
      }
    }
  }
}
```

**Scenario 3: Token Exchange Failure → Immediate 500 Response**

Ordinary exchange errors use this response:

```
ProcessingResponse{
  Response: ProcessingResponse_ImmediateResponse{
    ImmediateResponse: ImmediateResponse{
      Status: HttpStatus{Code: 500},
      Headers: HeaderMutation{
        SetHeaders: [
          HeaderValueOption{
            Header: HeaderValue{
              Key: "content-type",
              RawValue: "application/json"
            }
          }
        ]
      },
      Body: '{"error":"token_exchange_failed","error_description":"token exchange request failed"}'
    }
  }
}
```

Feature 043 defines the `error_uri`, expired-assertion, and circuit-open exceptions. No failure response emits an authorization mutation.


**Scenario 4: Invalid Metadata → Immediate 503 Response**

Feature 043 defines the metadata validation order and exact `invalid_subject_token` and `invalid_resource` JSON responses.

### RequestBody Phase → Pass Through

Body is not modified. The streaming response forwards the body unchanged.

```
ProcessingResponse{
  Response: ProcessingResponse_RequestBody{
    RequestBody: BodyResponse{
      Response: CommonResponse{
        BodyMutation: BodyMutation{
          Mutation: BodyMutation_StreamedResponse{
            StreamedResponse: StreamedBodyResponse{
              Body: <original_body>,
              EndOfStream: <original_end_of_stream>
            }
          }
        }
      }
    }
  }
}
```

### ResponseHeaders / ResponseBody / Trailers → Pass Through

Response phases are forwarded unchanged.

## HTTP Contracts (Outbound Calls)

### 1. Client Credentials Grant (OAuth2 Token Endpoint)

**Purpose**: Obtain an ID token to use as client assertion for token exchange.

**Request**:
```http
POST {oauth2.issuer}/oauth/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=client_credentials
&client_id={oauth2.client_id}
&client_secret={oauth2.client_secret}
&scope=openid
```

**Response** (200 OK):
```json
{
  "access_token": "<jwt>",
  "id_token": "<jwt>",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

The `id_token` from the response is used as the `client_assertion` (per RFC 7523). The `id_token` is a JWT asserting the client's identity to the identity broker. Note: ensure the upstream OAuth2 server is configured to return an `id_token` for `client_credentials` grants.

### 2. RFC 8693 Token Exchange (Identity Broker)

**Purpose**: Exchange the incoming Bearer token for a downstream service token.

**Request**:
```http
POST {oauth2.token_endpoint} HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn:ietf:params:oauth:grant-type:token-exchange
&subject_token={bearer_token}
&subject_token_type=urn:ietf:params:oauth:token-type:access_token
&resource={request_uri}
&client_assertion={id_token_jwt}
&client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer
```

**Response** (200 OK):
```json
{
  "access_token": "<exchanged_token>",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

**Error Responses**:

| HTTP Status | Error Code | Description |
|-------------|------------|-------------|
| 400 | `invalid_request` | Missing/invalid parameters |
| 400 | `invalid_target` | No service matches resource |
| 401 | `invalid_client` | Client assertion validation failed |
| 403 | `access_denied` | User has not granted agent access |
| 400 | `invalid_grant` | No active session for user/service |
