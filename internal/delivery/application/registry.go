// Package application coordinates delivery ports without importing an adapter.
package application

import (
	"fmt"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
)

// Registry exposes only carriers enabled for the current store deployment.
type Registry struct {
	carriers map[string]domain.Carrier
	default_ domain.Carrier
}

func NewRegistry(enabled []string, defaultCode string, carriers ...domain.Carrier) (*Registry, error) {
	expected := make(map[string]struct{}, len(enabled))
	for _, code := range enabled {
		code = normalize(code)
		if code == "" {
			return nil, fmt.Errorf("delivery provider code must not be empty")
		}
		if _, exists := expected[code]; exists {
			return nil, fmt.Errorf("delivery provider %q is configured more than once", code)
		}
		expected[code] = struct{}{}
	}

	defaultCode = normalize(defaultCode)
	if len(expected) == 0 {
		if defaultCode != "" {
			return nil, fmt.Errorf("delivery default requires an enabled carrier")
		}
		return &Registry{carriers: map[string]domain.Carrier{}}, nil
	}
	if defaultCode == "" {
		return nil, fmt.Errorf("delivery default carrier is required")
	}
	if _, enabled := expected[defaultCode]; !enabled {
		return nil, fmt.Errorf("delivery default carrier %q is not enabled", defaultCode)
	}

	registered := make(map[string]domain.Carrier, len(expected))
	for _, carrier := range carriers {
		if carrier == nil {
			return nil, fmt.Errorf("delivery carrier must not be nil")
		}
		code := normalize(carrier.Code())
		if _, enabled := expected[code]; !enabled {
			continue
		}
		if _, exists := registered[code]; exists {
			return nil, fmt.Errorf("delivery carrier %q is registered more than once", code)
		}
		registered[code] = carrier
	}
	for code := range expected {
		if _, exists := registered[code]; !exists {
			return nil, fmt.Errorf("enabled delivery carrier %q is not registered", code)
		}
	}

	return &Registry{carriers: registered, default_: registered[defaultCode]}, nil
}

func (r *Registry) Default() domain.Carrier {
	return r.default_
}

func (r *Registry) Get(code string) (domain.Carrier, bool) {
	carrier, ok := r.carriers[normalize(code)]
	return carrier, ok
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
