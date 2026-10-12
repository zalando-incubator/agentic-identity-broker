// Package http provides HTTP server adapters for the identity broker.
package middleware

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	domjwtauth "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtauth"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const (
	// maxPrincipalLength is the maximum allowed length for a principal value
	maxPrincipalLength = 200
)

// ErrorResponse represents a JSON error response structure.
type ErrorResponse struct {
	Error string `json:"error"`
}

// RequirePrincipalMiddleware returns middleware that requires a valid principal to be present.
// When a JWTAuthenticator is configured, it first attempts JWT-based authentication from the
// configured JWT header. If a JWT header is present, authentication is via JWT only (fail-closed,
// no fallback to plain header). If the JWT header is absent, the request is rejected with 401
// (fail-closed — no fallback to plain header when JWT is configured).
// If no JWTAuthenticator is configured, behavior is identical to the original plain-header mode.
//
// jwtAuth may be nil when JWT authentication is not configured.
//
// Returns:
// - 401 Unauthorized: When no authentication source provides a valid principal
// - 400 Bad Request: When the principal exceeds maximum length
// - Continues to next handler: When principal is valid
func RequirePrincipalMiddleware(authConfig ports.AuthenticationConfig, jwtAuth domjwtauth.JWTAuthenticator, logger *slog.Logger) func(next http.Handler) http.Handler {
	headerName := authConfig.Preauth.PrincipalHeaderName

	// Panic early if JWT is configured but no authenticator was injected — surface wiring bugs at startup.
	if authConfig.JWT != nil && jwtAuth == nil {
		panic("JWT authentication is configured but no JWTAuthenticator was injected: check server/app wiring")
	}

	authenticator := jwtAuth

	// Determine JWT header name from config
	jwtHeaderName := ""
	if authConfig.JWT != nil {
		jwtHeaderName = authConfig.JWT.HeaderName
		if jwtHeaderName == "" {
			jwtHeaderName = "Authorization"
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// JWT authentication path: when JWT authenticator is configured
			if authenticator != nil && jwtHeaderName != "" {
				jwtHeaderValue := r.Header.Get(jwtHeaderName)
				if jwtHeaderValue != "" {
					// JWT header is present — authenticate via JWT (fail-closed, no fallback)
					result, err := authenticator.Authenticate(r.Context(), jwtHeaderValue)
					if err != nil {
						if errors.Is(err, domjwtauth.ErrJWKSUnavailable) {
							logger.Error("JWKS endpoint unavailable (503 SERVICE UNAVAILABLE)",
								"error", err.Error(),
								"jwt_header", jwtHeaderName,
								"remote_addr", r.RemoteAddr,
								"path", r.URL.Path)
							writeErrorJSON(w, http.StatusServiceUnavailable, mapJWTError(err))
						} else {
							logger.Warn("JWT authentication failed (401 REJECT)",
								"error", err.Error(),
								"jwt_header", jwtHeaderName,
								"remote_addr", r.RemoteAddr,
								"path", r.URL.Path)
							writeErrorJSON(w, http.StatusUnauthorized, mapJWTError(err))
						}
						return
					}

					// Validate principal length
					if len(result.Principal) > maxPrincipalLength {
						err := principal.PrincipalTooLongError(len(result.Principal), maxPrincipalLength)
						logger.Warn("JWT principal exceeds maximum length",
							"length", len(result.Principal),
							"max_length", maxPrincipalLength,
							"remote_addr", r.RemoteAddr,
							"path", r.URL.Path)
						writeErrorJSON(w, http.StatusBadRequest, err.Error())
						return
					}

					// Set both principal (backward compat) and enriched profile in context
					ctx := principal.WithPrincipal(r.Context(), result.Principal)

					// Build PrincipalProfile from AuthResult
					profile := principal.NewProfile(result.Principal)
					if result.DisplayName != nil {
						profile = profile.WithDisplayName(*result.DisplayName)
					}
					if result.Email != nil {
						profile = profile.WithEmail(result.Email)
					}
					if result.PictureURL != nil {
						profile = profile.WithPictureURL(result.PictureURL)
					}
					ctx = principal.WithProfile(ctx, profile)
					r = r.WithContext(ctx)

					logger.Debug("JWT principal extracted",
						"jwt_header", jwtHeaderName,
						"principal", result.Principal,
						"has_display_name", result.DisplayName != nil,
						"has_email", result.Email != nil,
						"has_picture_url", result.PictureURL != nil,
						"remote_addr", r.RemoteAddr,
						"path", r.URL.Path)

					next.ServeHTTP(w, r)
					return
				}
				// JWT configured but header absent — fail closed, no fallback to plain header
				missingErr := principal.NewMissingPrincipalError(jwtHeaderName)
				logger.Warn("JWT authentication configured but JWT header absent (401 REJECT)",
					"header", jwtHeaderName,
					"remote_addr", r.RemoteAddr,
					"path", r.URL.Path)
				writeErrorJSON(w, http.StatusUnauthorized, missingErr.Error())
				return
			}

			// Plain header extraction path (original behavior)
			principalValue := r.Header.Get(headerName)
			principalValue = strings.TrimSpace(principalValue)

			if principalValue == "" {
				err := principal.NewMissingPrincipalError(headerName)
				logger.Warn("Missing or empty principal (401 REJECT)",
					"header", headerName,
					"remote_addr", r.RemoteAddr,
					"path", r.URL.Path)
				writeErrorJSON(w, http.StatusUnauthorized, err.Error())
				return
			}

			if len(principalValue) > maxPrincipalLength {
				err := principal.PrincipalTooLongError(len(principalValue), maxPrincipalLength)
				logger.Warn("Principal exceeds maximum length",
					"header", headerName,
					"length", len(principalValue),
					"max_length", maxPrincipalLength,
					"remote_addr", r.RemoteAddr,
					"path", r.URL.Path)
				writeErrorJSON(w, http.StatusBadRequest, err.Error())
				return
			}

			// Set both principal and basic profile (display name = principal) in context
			ctx := principal.WithPrincipal(r.Context(), principalValue)
			profile := principal.NewProfile(principalValue)
			ctx = principal.WithProfile(ctx, profile)
			r = r.WithContext(ctx)

			logger.Debug("Principal extracted",
				"header", headerName,
				"principal", principalValue,
				"remote_addr", r.RemoteAddr,
				"path", r.URL.Path)

			next.ServeHTTP(w, r)
		})
	}
}

// OptionalPrincipalMiddleware returns middleware that optionally extracts a principal.
// When a JWTAuthenticator is configured, it first attempts JWT-based authentication.
// If the JWT header is present and valid, the enriched profile is set in context.
// If the JWT header is present but invalid, the request continues without a principal.
// If the JWT header is absent and JWT is configured, continues without principal (no fallback to plain header).
// If no JWTAuthenticator is configured, uses plain header extraction.
// This middleware never rejects requests - it only adds principals if they are valid.
//
// jwtAuth may be nil when JWT authentication is not configured.
//
// Returns:
// - Continues to next handler: In all cases (principal may or may not be in context)
func OptionalPrincipalMiddleware(authConfig ports.AuthenticationConfig, jwtAuth domjwtauth.JWTAuthenticator, logger *slog.Logger) func(next http.Handler) http.Handler {
	headerName := authConfig.Preauth.PrincipalHeaderName
	if authConfig.JWT != nil && jwtAuth == nil {
		panic("JWT authentication is configured but no JWTAuthenticator was injected: check server/app wiring")
	}

	authenticator := jwtAuth

	// Determine JWT header name from config
	jwtHeaderName := ""
	if authConfig.JWT != nil {
		jwtHeaderName = authConfig.JWT.HeaderName
		if jwtHeaderName == "" {
			jwtHeaderName = "Authorization"
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// JWT authentication path (optional)
			if authenticator != nil && jwtHeaderName != "" {
				jwtHeaderValue := r.Header.Get(jwtHeaderName)
				if jwtHeaderValue != "" {
					result, err := authenticator.Authenticate(r.Context(), jwtHeaderValue)
					if err != nil {
						logger.Debug("Optional JWT authentication failed, continuing without principal",
							"error", err.Error(),
							"jwt_header", jwtHeaderName,
							"remote_addr", r.RemoteAddr,
							"path", r.URL.Path)
						// Optional: don't reject, continue without principal
						next.ServeHTTP(w, r)
						return
					}

					if len(result.Principal) <= maxPrincipalLength {
						ctx := principal.WithPrincipal(r.Context(), result.Principal)

						profile := principal.NewProfile(result.Principal)
						if result.DisplayName != nil {
							profile = profile.WithDisplayName(*result.DisplayName)
						}
						if result.Email != nil {
							profile = profile.WithEmail(result.Email)
						}
						if result.PictureURL != nil {
							profile = profile.WithPictureURL(result.PictureURL)
						}
						ctx = principal.WithProfile(ctx, profile)
						r = r.WithContext(ctx)

						logger.Debug("Optional JWT principal extracted",
							"jwt_header", jwtHeaderName,
							"principal", result.Principal,
							"remote_addr", r.RemoteAddr,
							"path", r.URL.Path)
					}

					next.ServeHTTP(w, r)
					return
				}
				// JWT configured but header absent — continue without principal, no fallback to plain header
				next.ServeHTTP(w, r)
				return
			}

			// Plain header extraction path
			principalValue := r.Header.Get(headerName)
			principalValue = strings.TrimSpace(principalValue)

			if principalValue != "" && len(principalValue) <= maxPrincipalLength {
				ctx := principal.WithPrincipal(r.Context(), principalValue)
				profile := principal.NewProfile(principalValue)
				ctx = principal.WithProfile(ctx, profile)
				r = r.WithContext(ctx)

				logger.Debug("Optional principal extracted",
					"header", headerName,
					"principal", principalValue,
					"remote_addr", r.RemoteAddr,
					"path", r.URL.Path)
			} else if principalValue != "" && len(principalValue) > maxPrincipalLength {
				logger.Debug("Optional principal ignored (exceeds max length)",
					"header", headerName,
					"length", len(principalValue),
					"max_length", maxPrincipalLength,
					"remote_addr", r.RemoteAddr,
					"path", r.URL.Path)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// mapJWTError translates domain JWT errors into user-facing error messages.
func mapJWTError(err error) string {
	switch {
	case errors.Is(err, domjwtauth.ErrJWKSUnavailable):
		return "JWKS endpoint unavailable"
	case errors.Is(err, domjwtauth.ErrInvalidSignature):
		return "invalid JWT signature"
	case errors.Is(err, domjwtauth.ErrTokenExpired):
		return "JWT token expired"
	case errors.Is(err, domjwtauth.ErrAudienceMismatch):
		return "JWT audience mismatch"
	case errors.Is(err, domjwtauth.ErrIssuerMismatch):
		return "JWT issuer mismatch"
	case errors.Is(err, domjwtauth.ErrMissingExpiry):
		return "JWT missing expiry"
	case errors.Is(err, domjwtauth.ErrMalformedToken):
		return "malformed JWT token"
	case errors.Is(err, domjwtauth.ErrClaimExtraction):
		return "JWT claim extraction failed"
	default:
		return "JWT authentication failed"
	}
}

// writeErrorJSON writes a JSON error response to the response writer.
// Sets Content-Type to application/json and writes the provided HTTP status code.
func writeErrorJSON(w http.ResponseWriter, statusCode int, errorMessage string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := ErrorResponse{Error: errorMessage}
	_ = json.NewEncoder(w).Encode(response)
}
