package cimd

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/urivalidation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Service orchestrates CIMD document fetch, validation, and caching.
type Service struct {
	fetcher       ports.CIMDFetcher
	cache         *CIMDCache
	nameBlocklist []string
	logger        *slog.Logger
}

// defaultReservedClientNames is always enforced regardless of operator configuration (FR-023c).
var defaultReservedClientNames = []string{
	"admin", "administrator", "system", "operator", "root", "superuser",
}

// NewService creates a new CIMD service.
// nameBlocklist is merged with defaultReservedClientNames so built-in terms are
// always enforced regardless of operator configuration (FR-023c).
func NewService(fetcher ports.CIMDFetcher, cache *CIMDCache, nameBlocklist []string, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	merged := append(append([]string(nil), defaultReservedClientNames...), nameBlocklist...)
	return &Service{
		fetcher:       fetcher,
		cache:         cache,
		nameBlocklist: merged,
		logger:        logger,
	}
}

// Resolve fetches and validates the CIMD document for the given URL and agent.
func (s *Service) Resolve(ctx context.Context, rawURL string, agent *storage.Agent) (*ClientIDMetadataDocument, error) {
	if err := urivalidation.ValidateCIMDClientURL(rawURL); err != nil {
		return nil, err
	}

	// RFC §3 SHOULD NOT: warn when client_id URL contains a query string
	if parsed, parseErr := url.Parse(rawURL); parseErr == nil && (parsed.RawQuery != "" || parsed.ForceQuery) {
		s.logger.WarnContext(ctx, "client_id URL contains query string (discouraged by RFC)",
			"url", rawURL)
	}

	// Cache hit
	if entry := s.cache.Get(rawURL); entry != nil {
		s.logger.DebugContext(ctx, "cimd_document_cache_hit", "url", rawURL)
		if err := entry.Document.ValidatePrivateUseRedirectURIs(agent.RedirectURIs); err != nil {
			s.logger.WarnContext(ctx, "cimd_document_invalid", "url", rawURL, "error", err)
			return nil, err
		}
		return entry.Document, nil
	}

	// Fetch from remote
	result, err := s.fetcher.Fetch(ctx, rawURL)
	if err != nil {
		var ssrfErr *ports.SSRFBlockedError
		if errors.As(err, &ssrfErr) {
			s.logger.WarnContext(ctx, "cimd_fetch_blocked", "url", rawURL, "error", err)
		} else {
			s.logger.WarnContext(ctx, "cimd_fetch_failed", "url", rawURL, "error", err)
		}
		return nil, err
	}

	// Parse and validate the raw response body
	doc, err := ParseDocument(result.Body, rawURL, s.nameBlocklist)
	if err != nil {
		s.logger.WarnContext(ctx, "cimd_document_invalid", "url", rawURL, "error", err)
		return nil, err
	}
	if err := doc.ValidatePrivateUseRedirectURIs(agent.RedirectURIs); err != nil {
		s.logger.WarnContext(ctx, "cimd_document_invalid", "url", rawURL, "error", err)
		return nil, err
	}

	// Brand pin check (informational — flow continues)
	if doc.ClientName != "" && agent.DisplayName != "" && doc.ClientName != agent.DisplayName {
		s.logger.WarnContext(ctx, "cimd_brand_pin_mismatch",
			"agent_id", agent.ID,
			"agent_display_name", agent.DisplayName,
			"cimd_client_name", doc.ClientName,
		)
	}

	s.cache.Set(rawURL, doc, CacheHeaders{CacheControl: result.CacheControl, Expires: result.Expires}, time.Now())

	s.logger.Info("cimd_document_fetched",
		"url", rawURL,
		"client_name", doc.ClientName,
		"redirect_uris_count", len(doc.RedirectURIs),
	)

	return doc, nil
}
