package bizevent

import (
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
)

type Definition struct {
	name         string
	ownerVisible bool
}

type Option func(*Definition)

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){2}$`)

var registry = struct {
	sync.Mutex
	definitions map[string]Definition
}{definitions: make(map[string]Definition)}

func MustDefine(name string, options ...Option) Definition {
	if !identifier.MatchString(name) {
		panic("invalid business event identifier: " + name)
	}
	configured := Definition{name: name}
	for _, option := range options {
		option(&configured)
	}
	d := Definition{name: name, ownerVisible: configured.ownerVisible}
	registry.Lock()
	defer registry.Unlock()
	if _, exists := registry.definitions[name]; exists {
		panic("duplicate business event identifier: " + name)
	}
	registry.definitions[name] = d
	return d
}

func OwnerVisible() Option {
	return func(d *Definition) { d.ownerVisible = true }
}
func (d Definition) Name() string         { return d.name }
func (d Definition) OwnerVisible() bool   { return d.ownerVisible }
func (d Definition) String() string       { return d.name }
func (d Definition) LogValue() slog.Value { return slog.StringValue(d.name) }
func Definitions() []Definition {
	registry.Lock()
	defer registry.Unlock()
	definitions := make([]Definition, 0, len(registry.definitions))
	for _, d := range registry.definitions {
		definitions = append(definitions, d)
	}
	slices.SortFunc(definitions, func(a, b Definition) int { return strings.Compare(a.name, b.name) })
	return definitions
}
