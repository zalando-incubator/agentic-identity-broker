package oauth2session

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const (
	DefaultSessionListLimit = 100
	MaxSessionListLimit     = 1000
	maxSessionPrincipalLen  = 200
	serviceIDTextLen        = 36
	maxSessionCursorBytes   = serviceIDTextLen + maxSessionPrincipalLen*utf8.UTFMax
)

var (
	ErrInvalidSessionQuery  = errors.New("invalid session query")
	ErrInvalidSessionLimit  = fmt.Errorf("%w: limit must be an integer from 1 to %d", ErrInvalidSessionQuery, MaxSessionListLimit)
	errInvalidSessionCursor = fmt.Errorf("%w: cursor is not valid", ErrInvalidSessionQuery)
)

// SessionListQuery selects one page of the operator listing.
// A zero Principal matches every user; otherwise the match is exact. A zero Limit means
// DefaultSessionListLimit. Cursor is the NextCursor of the previous page, or empty for the first page.
type SessionListQuery struct {
	Principal id.Principal
	Limit     int
	Cursor    string
}

// SessionList is one page of the operator listing. Truncated is true when more sessions matched
// than the page holds. NextCursor is set exactly when Truncated is true, and continues the listing.
type SessionList struct {
	Items      []*storage.SessionAdminSummary
	Truncated  bool
	NextCursor string
}

// SessionAdminService serves operator reads of third-party sessions across all users.
// It never touches token material.
type SessionAdminService struct {
	sessions ports.UserSessionAdminRepository
}

func NewSessionAdminService(sessions ports.UserSessionAdminRepository) *SessionAdminService {
	return &SessionAdminService{sessions: sessions}
}

// ListSessions lists sessions ordered by (principal, service_id), starting after the query cursor.
// The cursor is an opaque position. Because the order is fixed and the position is a key, a
// session created or deleted between pages never causes another session to repeat or vanish.
func (s *SessionAdminService) ListSessions(ctx context.Context, query SessionListQuery) (SessionList, error) {
	limit := query.Limit
	if limit == 0 {
		limit = DefaultSessionListLimit
	}
	if limit < 1 || limit > MaxSessionListLimit {
		return SessionList{}, ErrInvalidSessionLimit
	}
	if utf8.RuneCountInString(string(query.Principal)) > maxSessionPrincipalLen {
		return SessionList{}, fmt.Errorf("%w: principal exceeds %d characters", ErrInvalidSessionQuery, maxSessionPrincipalLen)
	}
	if !storableSessionText(string(query.Principal)) {
		return SessionList{}, fmt.Errorf("%w: principal must be valid UTF-8 without NUL characters", ErrInvalidSessionQuery)
	}
	var after storage.SessionAdminCursor
	if query.Cursor != "" {
		var err error
		if after, err = decodeSessionCursor(query.Cursor); err != nil {
			return SessionList{}, err
		}
		if !query.Principal.IsZero() && after.Principal != query.Principal {
			return SessionList{}, errInvalidSessionCursor
		}
	}
	items, err := s.sessions.ListAdminSummaries(ctx, query.Principal, after, limit+1)
	if err != nil {
		return SessionList{}, err
	}
	if items == nil {
		items = []*storage.SessionAdminSummary{}
	}
	list := SessionList{Items: items}
	if len(items) > limit {
		list.Items = items[:limit]
		list.Truncated = true
		last := list.Items[limit-1]
		list.NextCursor = encodeSessionCursor(storage.SessionAdminCursor{Principal: last.Principal, ServiceID: last.ServiceID})
	}
	return list, nil
}

// GetSession returns one session by ID. It returns ErrSessionNotFound when no session has that ID.
func (s *SessionAdminService) GetSession(ctx context.Context, sessionID id.SessionID) (*storage.SessionAdminSummary, error) {
	summary, err := s.sessions.GetAdminSummary(ctx, sessionID)
	if err != nil {
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindNotFound {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return summary, nil
}

func encodeSessionCursor(cursor storage.SessionAdminCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursor.ServiceID.String() + string(cursor.Principal)))
}

func decodeSessionCursor(raw string) (storage.SessionAdminCursor, error) {
	if len(raw) > base64.RawURLEncoding.EncodedLen(maxSessionCursorBytes) {
		return storage.SessionAdminCursor{}, errInvalidSessionCursor
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || len(decoded) <= serviceIDTextLen || !utf8.Valid(decoded) {
		return storage.SessionAdminCursor{}, errInvalidSessionCursor
	}
	serviceID, err := id.ParseServiceID(string(decoded[:serviceIDTextLen]))
	principal := id.Principal(decoded[serviceIDTextLen:])
	if err != nil || serviceID.String() != string(decoded[:serviceIDTextLen]) ||
		utf8.RuneCountInString(string(principal)) > maxSessionPrincipalLen || !storableSessionText(string(principal)) {
		return storage.SessionAdminCursor{}, errInvalidSessionCursor
	}
	return storage.SessionAdminCursor{Principal: principal, ServiceID: serviceID}, nil
}

// storableSessionText reports whether PostgreSQL text can hold s. It rejects invalid UTF-8 and NUL.
func storableSessionText(s string) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
