// Package application coordinates payment ports without importing an adapter.
package application

import (
	"fmt"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

// Registry exposes only gateways enabled for the current store deployment.
// Bootstrap supplies the adapters; this package never constructs SDK clients.
type Registry struct {
	gateways map[string]domain.Gateway
	default_ domain.Gateway
}

func NewRegistry(enabled []string, defaultCode string, gateways ...domain.Gateway) (*Registry, error) {
	expected := make(map[string]struct{}, len(enabled))
	for _, code := range enabled {
		code = normalize(code)
		if code == "" {
			return nil, fmt.Errorf("payment provider code must not be empty")
		}
		if _, exists := expected[code]; exists {
			return nil, fmt.Errorf("payment provider %q is configured more than once", code)
		}
		expected[code] = struct{}{}
	}

	defaultCode = normalize(defaultCode)
	if len(expected) == 0 {
		if defaultCode != "" {
			return nil, fmt.Errorf("payment default requires an enabled gateway")
		}
		return &Registry{gateways: map[string]domain.Gateway{}}, nil
	}
	if defaultCode == "" {
		return nil, fmt.Errorf("payment default gateway is required")
	}
	if _, enabled := expected[defaultCode]; !enabled {
		return nil, fmt.Errorf("payment default gateway %q is not enabled", defaultCode)
	}

	registered := make(map[string]domain.Gateway, len(expected))
	for _, gateway := range gateways {
		if gateway == nil {
			return nil, fmt.Errorf("payment gateway must not be nil")
		}
		code := normalize(gateway.Code())
		if _, enabled := expected[code]; !enabled {
			continue
		}
		if _, exists := registered[code]; exists {
			return nil, fmt.Errorf("payment gateway %q is registered more than once", code)
		}
		registered[code] = gateway
	}
	for code := range expected {
		if _, exists := registered[code]; !exists {
			return nil, fmt.Errorf("enabled payment gateway %q is not registered", code)
		}
	}

	return &Registry{gateways: registered, default_: registered[defaultCode]}, nil
}

func (r *Registry) Default() domain.Gateway {
	return r.default_
}

func (r *Registry) Get(code string) (domain.Gateway, bool) {
	provider, ok := r.gateways[normalize(code)]
	return provider, ok
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
