package google

import (
	"context"
	"net/url"
	"testing"

	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

func TestBeginAuthorizationUsesStateNonceAndPKCE(t *testing.T) {
	adapter, err := New(Config{ClientID: "client-id", ClientSecret: "client-secret", AllowedRedirectURIs: []string{"https://store.example.test/api/auth/google/callback"}})
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := adapter.BeginAuthorization(context.Background(), identityDomain.OAuthAuthorizationRequest{RedirectURI: "https://store.example.test/api/auth/google/callback", State: "state-value", Nonce: "nonce-value", CodeVerifier: "a-code-verifier-with-enough-length-to-be-valid-123"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authorization.RedirectURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("state") != "state-value" || query.Get("nonce") != "nonce-value" || query.Get("code_challenge") == "" || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("Google authorization query = %s", parsed.RawQuery)
	}
}
