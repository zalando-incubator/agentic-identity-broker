package servicescope

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ServiceLookup resolves services by UUID or current canonical ID.
type ServiceLookup interface {
	Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error)
	GetByCanonicalID(ctx context.Context, canonicalID string) (*model.ThirdpartyOAuth2ProviderEntity, error)
}

// PermissionSetLookup batch-loads permission sets; missing IDs are absent from the result.
type PermissionSetLookup interface {
	GetByIDs(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error)
}

// RequestedService is one service named by one or more Virtual Service Scopes.
type RequestedService struct {
	ServiceID id.ServiceID
	Service   *model.ThirdpartyOAuth2ProviderEntity
	Scopes    []VirtualServiceScope
}

// RequestedServiceSet is the ordered, duplicate-free set of requested services plus the
// Service-Scope Bindings of every resolved raw scope.
type RequestedServiceSet struct {
	Services []RequestedService
	Bindings map[string]id.ServiceID
}

// Empty reports whether no service was requested.
func (s *RequestedServiceSet) Empty() bool {
	return s == nil || len(s.Services) == 0
}

// ServiceIDs returns the requested service IDs in request order.
func (s *RequestedServiceSet) ServiceIDs() []id.ServiceID {
	if s == nil {
		return nil
	}
	ids := make([]id.ServiceID, len(s.Services))
	for i, service := range s.Services {
		ids[i] = service.ServiceID
	}
	return ids
}

// Resolver resolves Virtual Service Scopes for an agent.
type Resolver struct {
	services       ServiceLookup
	permissionSets PermissionSetLookup
	publicURL      string
}

// NewResolver creates a Resolver for the configured end-user public URL.
func NewResolver(services ServiceLookup, permissionSets PermissionSetLookup, publicURL string) *Resolver {
	if services == nil || permissionSets == nil {
		panic("servicescope.NewResolver: lookups must not be nil")
	}
	return &Resolver{services: services, permissionSets: permissionSets, publicURL: strings.TrimRight(publicURL, "/")}
}

// PublicURL returns the configured origin used for parsing and emission.
func (r *Resolver) PublicURL() string {
	return r.publicURL
}

// Format emits the current Virtual Service Scope of a service.
func (r *Resolver) Format(service *model.ThirdpartyOAuth2ProviderEntity) string {
	return Format(r.publicURL, service.ID, service.CanonicalID)
}

// Resolve parses the space-delimited scope and resolves its broker scopes. Ordinary tokens are
// ignored and cost no I/O. With non-nil bindings (a sealed or persisted continuation), every
// virtual scope must carry a binding that still resolves to the same UUID.
func (r *Resolver) Resolve(ctx context.Context, scope string, agent *storage.Agent, bindings map[string]id.ServiceID) (*RequestedServiceSet, error) {
	var requested []VirtualServiceScope
	for _, token := range strings.Fields(scope) {
		parsed, kind := Parse(token, r.publicURL)
		switch kind {
		case KindMalformed:
			return nil, ErrMalformedScope
		case KindVirtual:
			requested = append(requested, parsed)
		}
	}
	if len(requested) == 0 {
		return &RequestedServiceSet{}, nil
	}
	if bindings != nil {
		for _, parsed := range requested {
			if _, ok := bindings[parsed.Raw]; !ok {
				return nil, ErrBindingMismatch
			}
		}
	}
	return r.resolve(ctx, requested, agent, bindings)
}

// ResolveBindings rechecks a complete assurance binding set, including bindings whose scope
// was narrowed away from the current grant.
func (r *Resolver) ResolveBindings(ctx context.Context, bindings map[string]id.ServiceID, agent *storage.Agent) (*RequestedServiceSet, error) {
	requested := make([]VirtualServiceScope, 0, len(bindings))
	for _, raw := range slices.Sorted(maps.Keys(bindings)) {
		parsed, kind := Parse(raw, r.publicURL)
		if kind != KindVirtual {
			return nil, ErrBindingMismatch
		}
		requested = append(requested, parsed)
	}
	return r.resolve(ctx, requested, agent, bindings)
}

func (r *Resolver) resolve(ctx context.Context, requested []VirtualServiceScope, agent *storage.Agent, bindings map[string]id.ServiceID) (*RequestedServiceSet, error) {
	set := &RequestedServiceSet{Bindings: make(map[string]id.ServiceID, len(requested))}
	index := map[id.ServiceID]int{}
	var eligible map[id.ServiceID]bool
	for _, parsed := range requested {
		if _, seen := set.Bindings[parsed.Raw]; seen {
			continue
		}
		service, err := r.lookup(ctx, parsed)
		if err != nil {
			return nil, err
		}
		if bound, ok := bindings[parsed.Raw]; ok && bound != service.ID {
			return nil, ErrBindingMismatch
		}
		if eligible == nil {
			if eligible, err = r.eligibleServices(ctx, agent); err != nil {
				return nil, err
			}
		}
		if !eligible[service.ID] {
			return nil, ErrServiceNotEligible
		}
		set.Bindings[parsed.Raw] = service.ID
		if position, ok := index[service.ID]; ok {
			set.Services[position].Scopes = append(set.Services[position].Scopes, parsed)
			continue
		}
		index[service.ID] = len(set.Services)
		set.Services = append(set.Services, RequestedService{ServiceID: service.ID, Service: service, Scopes: []VirtualServiceScope{parsed}})
	}
	return set, nil
}

func (r *Resolver) lookup(ctx context.Context, parsed VirtualServiceScope) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	var (
		service *model.ThirdpartyOAuth2ProviderEntity
		err     error
	)
	if parsed.Form == FormUUID {
		serviceID, parseErr := id.ParseServiceID(parsed.Identifier)
		if parseErr != nil {
			return nil, ErrMalformedScope
		}
		service, err = r.services.Get(ctx, serviceID)
	} else {
		service, err = r.services.GetByCanonicalID(ctx, parsed.Identifier)
	}
	if err != nil {
		if ports.IsNotFoundErr(err) {
			return nil, ErrUnknownService
		}
		return nil, fmt.Errorf("resolve service scope: %w", err)
	}
	if service == nil {
		return nil, ErrUnknownService
	}
	return service, nil
}

// eligibleServices returns the services the agent may request: those in its service
// requirements when it declares any, otherwise those covered by its declared permission sets.
func (r *Resolver) eligibleServices(ctx context.Context, agent *storage.Agent) (map[id.ServiceID]bool, error) {
	eligible := map[id.ServiceID]bool{}
	if len(agent.ServiceRequirements) > 0 {
		for _, requirement := range agent.ServiceRequirements {
			eligible[requirement.ServiceID] = true
		}
		return eligible, nil
	}
	declared := make([]id.PermissionSetID, len(agent.PermissionSets))
	for i, entry := range agent.PermissionSets {
		declared[i] = entry.PermissionSetID
	}
	if len(declared) == 0 {
		return eligible, nil
	}
	sets, err := r.permissionSets.GetByIDs(ctx, declared)
	if err != nil {
		return nil, fmt.Errorf("resolve declared permission sets: %w", err)
	}
	for _, set := range sets {
		for _, scope := range set.ServiceScopes {
			eligible[scope.ServiceID] = true
		}
	}
	return eligible, nil
}
