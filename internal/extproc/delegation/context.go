package delegation

import (
	"encoding/json"
	"errors"
	"time"
)

var errInvalidContext = errors.New("permit context violates the documented shape")

// Wire types decode the permit context. Unknown members are ignored at every level and
// never copied. Pointers and raw optional members distinguish absent, null and empty values.
type wireContext struct {
	Subject *struct {
		ID *string `json:"id"`
	} `json:"subject"`
	Agent          *wireEntity           `json:"agent"`
	Resource       *wireResource         `json:"resource"`
	Grant          *wireGrant            `json:"grant"`
	PermissionSets *[]*wirePermissionSet `json:"permission_sets"`
}

type wireEntity struct {
	ID          *string         `json:"id"`
	CanonicalID json.RawMessage `json:"canonical_id"`
}

type wireResource struct {
	ID          *string         `json:"id"`
	CanonicalID json.RawMessage `json:"canonical_id"`
	ResourceURI *string         `json:"resource_uri"`
	Scopes      *[]string       `json:"scopes"`
}

type wireGrant struct {
	ID         *string         `json:"id"`
	UpdatedAt  *string         `json:"updated_at"`
	ValidUntil json.RawMessage `json:"valid_until"`
}

type wirePermissionSet struct {
	ID          *string         `json:"id"`
	CanonicalID json.RawMessage `json:"canonical_id"`
	Services    *[]*wireService `json:"services"`
}

type wireService struct {
	ID          *string         `json:"id"`
	CanonicalID json.RawMessage `json:"canonical_id"`
	Scopes      *[]string       `json:"scopes"`
}

func parsePermitContext(raw json.RawMessage) (Context, error) {
	if len(raw) == 0 {
		return Context{}, errInvalidContext
	}
	var wire *wireContext
	if err := json.Unmarshal(raw, &wire); err != nil || wire == nil {
		return Context{}, errInvalidContext
	}
	if wire.Subject == nil || wire.Agent == nil || wire.Resource == nil || wire.Grant == nil || wire.PermissionSets == nil {
		return Context{}, errInvalidContext
	}
	principal, ok := required(wire.Subject.ID)
	if !ok {
		return Context{}, errInvalidContext
	}
	result := Context{Principal: principal}

	if result.Agent, ok = entity(wire.Agent.ID, wire.Agent.CanonicalID); !ok {
		return Context{}, errInvalidContext
	}

	resourceRef, ok := entity(wire.Resource.ID, wire.Resource.CanonicalID)
	if !ok {
		return Context{}, errInvalidContext
	}
	resourceURI, ok := required(wire.Resource.ResourceURI)
	if !ok || wire.Resource.Scopes == nil {
		return Context{}, errInvalidContext
	}
	result.Resource = Resource{ID: resourceRef.ID, CanonicalID: resourceRef.CanonicalID, ResourceURI: resourceURI,
		Scopes: append([]string{}, *wire.Resource.Scopes...)}

	grantID, ok := required(wire.Grant.ID)
	if !ok {
		return Context{}, errInvalidContext
	}
	updatedAt, ok := required(wire.Grant.UpdatedAt)
	if !ok || !isRFC3339(updatedAt) {
		return Context{}, errInvalidContext
	}
	validUntil, ok := optional(wire.Grant.ValidUntil)
	if !ok || (validUntil != "" && !isRFC3339(validUntil)) {
		return Context{}, errInvalidContext
	}
	result.Grant = Grant{ID: grantID, UpdatedAt: updatedAt, ValidUntil: validUntil}

	if len(*wire.PermissionSets) == 0 {
		return Context{}, errInvalidContext
	}
	result.PermissionSets = make([]PermissionSet, 0, len(*wire.PermissionSets))
	for _, wirePS := range *wire.PermissionSets {
		if wirePS == nil || wirePS.Services == nil {
			return Context{}, errInvalidContext
		}
		ref, ok := entity(wirePS.ID, wirePS.CanonicalID)
		if !ok {
			return Context{}, errInvalidContext
		}
		ps := PermissionSet{ID: ref.ID, CanonicalID: ref.CanonicalID, Services: make([]Service, 0, len(*wirePS.Services))}
		for _, wireSvc := range *wirePS.Services {
			if wireSvc == nil || wireSvc.Scopes == nil {
				return Context{}, errInvalidContext
			}
			svcRef, ok := entity(wireSvc.ID, wireSvc.CanonicalID)
			if !ok {
				return Context{}, errInvalidContext
			}
			ps.Services = append(ps.Services, Service{ID: svcRef.ID, CanonicalID: svcRef.CanonicalID, Scopes: append([]string{}, *wireSvc.Scopes...)})
		}
		result.PermissionSets = append(result.PermissionSets, ps)
	}
	return result, nil
}

func entity(idValue *string, canonicalID json.RawMessage) (EntityRef, bool) {
	identifier, ok := required(idValue)
	if !ok {
		return EntityRef{}, false
	}
	canonical, ok := optional(canonicalID)
	if !ok {
		return EntityRef{}, false
	}
	return EntityRef{ID: identifier, CanonicalID: canonical}, true
}

func required(value *string) (string, bool) {
	if value == nil || *value == "" {
		return "", false
	}
	return *value, true
}

// optional accepts an absent member or a non-empty string; explicit null and "" contradict
// the contract.
func optional(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", true
	}
	var value *string
	if err := json.Unmarshal(raw, &value); err != nil || value == nil || *value == "" {
		return "", false
	}
	return *value, true
}

func isRFC3339(value string) bool {
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}
