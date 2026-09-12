// Package unsubscribe turns Consent's signed opt-out capability into the link a
// marketing message carries.
//
// It lives with Consent because Consent owns the token. The modules that send
// marketing depend on the narrow UnsubscribeLinker port instead of on this.
package unsubscribe

import (
	"fmt"
	"net/url"
	"strings"

	consentApp "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/application"
)

type Linker struct {
	service *consentApp.Service
	base    string
}

// NewLinker builds links under the storefront's own URL, not the API's: the
// recipient clicks into a page the store renders, and that page posts the
// token. The endpoint is POST precisely because mail security scanners follow
// links, and a GET would let a provider's link check unsubscribe the customer.
func NewLinker(service *consentApp.Service, frontendURL string) (*Linker, error) {
	base := strings.TrimRight(strings.TrimSpace(frontendURL), "/")
	if service == nil || base == "" {
		return nil, fmt.Errorf("unsubscribe linker requires the consent service and FRONTEND_URL")
	}
	if _, err := url.ParseRequestURI(base); err != nil {
		return nil, fmt.Errorf("FRONTEND_URL is not a usable base for an unsubscribe link: %w", err)
	}
	return &Linker{service: service, base: base}, nil
}

func (l *Linker) URLFor(email string) (string, error) {
	token, err := l.service.UnsubscribeToken(email)
	if err != nil {
		return "", fmt.Errorf("mint unsubscribe token: %w", err)
	}
	return l.base + "/unsubscribe?token=" + url.QueryEscape(token), nil
}
