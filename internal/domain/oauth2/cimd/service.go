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
		panic("cimd.NewService: logger must not be nil")
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
			"agent_id", agent.ID)
	}

	// Cache hit
	if entry := s.cache.Get(rawURL); entry != nil {
		s.logger.DebugContext(ctx, "cimd_document_cache_hit", "agent_id", agent.ID)
		if err := entry.Document.ValidatePrivateUseRedirectURIs(agent.RedirectURIs); err != nil {
			s.logFailure(ctx, "cimd_document_invalid", agent, ErrorKindRedirectURINotAllowlisted)
			return nil, err
		}
		return entry.Document, nil
	}

	// Fetch from remote
	result, err := s.fetcher.Fetch(ctx, rawURL)
	if err != nil {
		kind := fetchErrorKind(err)
		if kind == ErrorKindSSRFBlocked {
			s.logFailure(ctx, "cimd_fetch_blocked", agent, kind)
		} else {
			s.logFailure(ctx, "cimd_fetch_failed", agent, kind)
		}
		return nil, err
	}

	// Parse and validate the raw response body
	doc, err := ParseDocument(result.Body, rawURL, s.nameBlocklist)
	if err != nil {
		s.logFailure(ctx, "cimd_document_invalid", agent, ErrorKindInvalidDocument)
		return nil, err
	}
	if err := doc.ValidatePrivateUseRedirectURIs(agent.RedirectURIs); err != nil {
		s.logFailure(ctx, "cimd_document_invalid", agent, ErrorKindRedirectURINotAllowlisted)
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

	s.logger.InfoContext(ctx, "cimd_document_fetched",
		"agent_id", agent.ID,
		"client_name", doc.ClientName,
		"redirect_uris_count", len(doc.RedirectURIs),
	)

	return doc, nil
}

// CIMD resolution error kinds. A fetch or validation error can carry the client_id URL
// query (*url.Error) or document content, so only the kind is logged.
const (
	ErrorKindSSRFBlocked               = "ssrf_blocked"
	ErrorKindUnavailable               = "unavailable"
	ErrorKindFetchFailed               = "fetch_failed"
	ErrorKindInvalidDocument           = "invalid_document"
	ErrorKindRedirectURINotAllowlisted = "redirect_uri_not_allowlisted"
)

func fetchErrorKind(err error) string {
	var ssrfErr *ports.SSRFBlockedError
	if errors.As(err, &ssrfErr) {
		return ErrorKindSSRFBlocked
	}
	var unavailable *ports.CIMDUnavailableError
	if errors.As(err, &unavailable) {
		return ErrorKindUnavailable
	}
	return ErrorKindFetchFailed
}

func (s *Service) logFailure(ctx context.Context, msg string, agent *storage.Agent, kind string) {
	s.logger.WarnContext(ctx, msg, "agent_id", agent.ID, "error_kind", kind)
}
