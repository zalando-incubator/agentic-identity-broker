package memory

import (
	"maps"
	"reflect"
	"slices"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func copyPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyJSONValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		if value == nil {
			return map[string]any(nil)
		}
		copy := make(map[string]any, len(value))
		for key, item := range value {
			copy[key] = copyJSONValue(item)
		}
		return copy
	case []any:
		if value == nil {
			return []any(nil)
		}
		copy := make([]any, len(value))
		for i, item := range value {
			copy[i] = copyJSONValue(item)
		}
		return copy
	case []string:
		return slices.Clone(value)
	case map[string]string:
		return maps.Clone(value)
	default:
		items := reflect.ValueOf(value)
		if items.IsValid() && items.Kind() == reflect.Slice && !items.IsNil() {
			copy := reflect.MakeSlice(items.Type(), items.Len(), items.Len())
			reflect.Copy(copy, items)
			return copy.Interface()
		}
		return value
	}
}

func copyUserSession(session *storage.UserSession) *storage.UserSession {
	copy := *session
	copy.EncryptedAccessToken = slices.Clone(session.EncryptedAccessToken)
	copy.EncryptedRefreshToken = slices.Clone(session.EncryptedRefreshToken)
	copy.Scope = slices.Clone(session.Scope)
	copy.AccessTokenExpiresAt = copyPointer(session.AccessTokenExpiresAt)
	copy.RefreshTokenExpiresAt = copyPointer(session.RefreshTokenExpiresAt)
	return &copy
}

func copyAuthorizationCode(code *storage.AuthorizationCode) *storage.AuthorizationCode {
	copy := *code
	copy.Email = copyPointer(code.Email)
	copy.UsedAt = copyPointer(code.UsedAt)
	return &copy
}

func copyRefreshTokenSession(session *storage.RefreshTokenSession) *storage.RefreshTokenSession {
	copy := *session
	copy.Email = copyPointer(session.Email)
	copy.UsedAt = copyPointer(session.UsedAt)
	return &copy
}

func copyClientCredential(credential *storage.ClientCredential) *storage.ClientCredential {
	copy := *credential
	copy.RotatedAt = copyPointer(credential.RotatedAt)
	return &copy
}
