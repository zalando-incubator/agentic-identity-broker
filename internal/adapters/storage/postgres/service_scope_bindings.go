package postgres

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// serviceScopeBindingsColumn maps nullable service_scope_bindings JSONB to domain bindings.
type serviceScopeBindingsColumn storage.ServiceScopeBindings

func (c serviceScopeBindingsColumn) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil
	}
	encoded := make(map[string]string, len(c))
	for scope, serviceID := range c {
		encoded[scope] = serviceID.String()
	}
	return json.Marshal(encoded)
}

func (c *serviceScopeBindingsColumn) Scan(value any) error {
	if value == nil {
		*c = nil
		return nil
	}
	var raw []byte
	switch typed := value.(type) {
	case []byte:
		raw = typed
	case string:
		raw = []byte(typed)
	default:
		return fmt.Errorf("unsupported service_scope_bindings type %T", value)
	}
	var decoded map[string]string
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("decode service_scope_bindings: %w", err)
	}
	bindings := make(serviceScopeBindingsColumn, len(decoded))
	for scope, serviceID := range decoded {
		parsed, err := id.ParseServiceID(serviceID)
		if err != nil {
			return fmt.Errorf("decode service_scope_bindings: %w", err)
		}
		bindings[scope] = parsed
	}
	*c = bindings
	return nil
}

type authorizationCodeRow struct {
	storage.AuthorizationCode
	Bindings serviceScopeBindingsColumn `db:"service_scope_bindings"`
}

type refreshTokenSessionRow struct {
	storage.RefreshTokenSession
	Bindings serviceScopeBindingsColumn `db:"service_scope_bindings"`
}
