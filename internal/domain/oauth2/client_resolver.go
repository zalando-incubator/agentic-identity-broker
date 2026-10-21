package oauth2

import (
	"context"
	"errors"
	"log/slog"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/cimd"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/urivalidation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// AgentClientResolver resolves client_id values to an Agent.
// Constructed via NewAgentClientResolver (opaque UUID-only) or
// NewAgentClientResolverWithCIMD (CIMD-enabled).
type AgentClientResolver struct {
	agentRepo   ports.AgentRepository
	cimdService *cimd.Service
	logger      *slog.Logger
}

// NewAgentClientResolver creates a resolver for opaque (UUID-based) client IDs.
// URL-format client_id values are rejected with invalid_client.
// When logger is nil, slog.Default() is used.
func NewAgentClientResolver(agentRepo ports.AgentRepository, logger *slog.Logger) *AgentClientResolver {
	if logger == nil {
		logger = slog.Default()
	}
	return &AgentClientResolver{agentRepo: agentRepo, logger: logger}
}

// NewAgentClientResolverWithCIMD creates a CIMD-enabled resolver.
// URL-format client_id values are resolved via CIMD fetch/validate/cache;
// non-URL client_id values fall through to opaque UUID lookup.
// When logger is nil, slog.Default() is used.
func NewAgentClientResolverWithCIMD(agentRepo ports.AgentRepository, cimdService *cimd.Service, logger *slog.Logger) *AgentClientResolver {
	if logger == nil {
		logger = slog.Default()
	}
	return &AgentClientResolver{agentRepo: agentRepo, cimdService: cimdService, logger: logger}
}

// ResolveClient resolves a client_id to an Agent and optional CIMD metadata.
// For URL-format client_id: validates URL, looks up agent by client URI, fetches/validates CIMD.
// For opaque client_id: parses UUID and looks up agent by ID.
func (r *AgentClientResolver) ResolveClient(ctx context.Context, clientID id.ClientID) (*ports.ClientResolution, error) {
	if urivalidation.ValidateCIMDClientURL(string(clientID)) == nil {
		return r.resolveCIMD(ctx, string(clientID))
	}
	return r.resolveOpaque(ctx, clientID)
}

func (r *AgentClientResolver) resolveCIMD(ctx context.Context, rawURL string) (*ports.ClientResolution, error) {
	if r.cimdService == nil {
		return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "URL-format client_id requires CIMD support (disabled)"}
	}

	// Look up agent by pre-registered client URI (FR-026)
	agent, err := r.agentRepo.GetByClientURI(ctx, rawURL)
	if err != nil {
		if ports.IsNotFoundErr(err) {
			return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "Client not registered"}
		}
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindConflict {
			r.logger.WarnContext(ctx, "cimd_client_uri_ambiguous", "client_uri", rawURL)
			return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "Client not registered"}
		}
		r.logger.ErrorContext(ctx, "failed to look up agent by client URI", "error", err, "client_uri", rawURL)
		return nil, &ports.ClientIDError{Code: "server_error", Desc: "Failed to validate client"}
	}

	// Fetch and validate CIMD document
	doc, err := r.cimdService.Resolve(ctx, rawURL, agent)
	if err != nil {
		var unavailable *ports.CIMDUnavailableError
		if errors.As(err, &unavailable) {
			r.logger.ErrorContext(ctx, "CIMD document temporarily unavailable")
			return nil, &ports.ClientIDError{Code: "server_error", Desc: "CIMD document unavailable"}
		}
		r.logger.WarnContext(ctx, "CIMD document validation failed", "error", err, "client_uri", rawURL)
		return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "CIMD document validation failed"}
	}

	return ports.NewClientResolution(agent, toCIMDMetadataDTO(doc)), nil
}

func (r *AgentClientResolver) resolveOpaque(ctx context.Context, clientID id.ClientID) (*ports.ClientResolution, error) {
	agentUUID, parseErr := id.ParseAgentID(string(clientID))
	if parseErr != nil {
		return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "Client not registered"}
	}

	agent, err := r.agentRepo.Get(ctx, agentUUID)
	if err != nil {
		if ports.IsNotFoundErr(err) {
			return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "Client not registered"}
		}
		r.logger.ErrorContext(ctx, "failed to look up agent by ID", "error", err, "agent_id", agentUUID)
		return nil, &ports.ClientIDError{Code: "server_error", Desc: "Failed to validate client"}
	}

	// CIMD and ambiguous agents must not be addressed by bare UUID when CIMD is disabled.
	// CIMDClient agents require URL-form client_id with CIMD document validation.
	// AmbiguousClient agents are always invalid.
	switch agent.ClientType() {
	case storage.CIMDClient:
		desc := "Client requires CIMD support (disabled)"
		if r.cimdService != nil {
			desc = "CIMD client must use URL-form client_id"
		}
		return nil, &ports.ClientIDError{Code: "invalid_client", Desc: desc}
	case storage.AmbiguousClient:
		return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "Client not registered"}
	}

	return ports.NewClientResolution(agent, nil), nil
}

// toCIMDMetadataDTO converts a domain ClientIDMetadataDocument to the port DTO.
func toCIMDMetadataDTO(doc *cimd.ClientIDMetadataDocument) *ports.CIMDMetadataDTO {
	if doc == nil {
		return nil
	}
	keySet := doc.KeySet
	if keySet != nil {
		if cloned, err := keySet.Clone(); err == nil {
			keySet = cloned
		} else {
			keySet = nil
		}
	}
	return &ports.CIMDMetadataDTO{
		ClientID:      doc.ClientID,
		ClientName:    doc.ClientName,
		LogoURI:       doc.LogoURI,
		RedirectURIs:  append([]string(nil), doc.RedirectURIs...),
		AuthMethod:    doc.AuthMethod,
		SigningAlg:    doc.SigningAlg,
		JWKSURI:       doc.JWKSURI,
		KeySet:        keySet,
		GrantTypes:    append([]string(nil), doc.GrantTypes...),
		ResponseTypes: append([]string(nil), doc.ResponseTypes...),
		PolicyURI:     doc.PolicyURI,
		TosURI:        doc.TosURI,
	}
}
