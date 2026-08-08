package service

import (
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

// OAuthProviderRegistry is constructed by Bootstrap from configured adapters.
type OAuthProviderRegistry struct {
	providers map[string]domain.OAuthProvider
}

func NewOAuthProviderRegistry(providers ...domain.OAuthProvider) *OAuthProviderRegistry {
	registry := &OAuthProviderRegistry{providers: make(map[string]domain.OAuthProvider, len(providers))}
	for _, provider := range providers {
		if provider != nil {
			registry.providers[strings.ToLower(strings.TrimSpace(provider.Code()))] = provider
		}
	}
	return registry
}

func (r *OAuthProviderRegistry) Get(code string) (domain.OAuthProvider, bool) {
	provider, ok := r.providers[strings.ToLower(strings.TrimSpace(code))]
	return provider, ok
}

var _ domain.OAuthProviderRegistry = (*OAuthProviderRegistry)(nil)
