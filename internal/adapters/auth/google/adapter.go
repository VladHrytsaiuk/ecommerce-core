// Package google adapts Google OpenID Connect to the provider-neutral identity
// OAuthProvider port. It contains no account-linking, JWT, or profile logic.
package google

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
	googleOAuth "golang.org/x/oauth2/google"
	googleIDToken "google.golang.org/api/idtoken"

	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

const code = "google"

type Config struct {
	ClientID            string
	ClientSecret        string
	AllowedRedirectURIs []string
	HTTPClient          *http.Client
}

type Adapter struct {
	clientID     string
	clientSecret string
	redirectURIs map[string]struct{}
	httpClient   *http.Client
	validate     func(context.Context, string, string) (*googleIDToken.Payload, error)
}

func New(config Config) (*Adapter, error) {
	if strings.TrimSpace(config.ClientID) == "" || strings.TrimSpace(config.ClientSecret) == "" || len(config.AllowedRedirectURIs) == 0 {
		return nil, fmt.Errorf("Google OAuth client id, client secret and allowed redirect URIs are required")
	}
	redirectURIs := make(map[string]struct{}, len(config.AllowedRedirectURIs))
	for _, redirectURI := range config.AllowedRedirectURIs {
		if redirectURI = strings.TrimSpace(redirectURI); redirectURI != "" {
			redirectURIs[redirectURI] = struct{}{}
		}
	}
	if len(redirectURIs) == 0 {
		return nil, fmt.Errorf("Google OAuth allowed redirect URIs are required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	return &Adapter{clientID: strings.TrimSpace(config.ClientID), clientSecret: strings.TrimSpace(config.ClientSecret), redirectURIs: redirectURIs, httpClient: config.HTTPClient, validate: googleIDToken.Validate}, nil
}

func (*Adapter) Code() string { return code }

func (a *Adapter) BeginAuthorization(_ context.Context, request identityDomain.OAuthAuthorizationRequest) (identityDomain.OAuthAuthorization, error) {
	if !a.allowsRedirectURI(request.RedirectURI) || strings.TrimSpace(request.State) == "" || strings.TrimSpace(request.Nonce) == "" || strings.TrimSpace(request.CodeVerifier) == "" {
		return identityDomain.OAuthAuthorization{}, fmt.Errorf("Google OAuth authorization request is incomplete")
	}
	config := a.oauthConfig(request.RedirectURI)
	url := config.AuthCodeURL(request.State,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(request.CodeVerifier),
		oauth2.SetAuthURLParam("nonce", request.Nonce),
	)
	return identityDomain.OAuthAuthorization{RedirectURL: url}, nil
}

func (a *Adapter) ExchangeCode(ctx context.Context, request identityDomain.OAuthCodeExchange) (identityDomain.VerifiedOAuthIdentity, error) {
	if !a.allowsRedirectURI(request.RedirectURI) || strings.TrimSpace(request.Code) == "" || strings.TrimSpace(request.CodeVerifier) == "" || strings.TrimSpace(request.Nonce) == "" {
		return identityDomain.VerifiedOAuthIdentity{}, fmt.Errorf("Google OAuth code exchange is incomplete")
	}
	config := a.oauthConfig(request.RedirectURI)
	if a.httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, a.httpClient)
	}
	token, err := config.Exchange(ctx, request.Code, oauth2.VerifierOption(request.CodeVerifier))
	if err != nil {
		return identityDomain.VerifiedOAuthIdentity{}, fmt.Errorf("exchange Google OAuth code: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || strings.TrimSpace(rawIDToken) == "" {
		return identityDomain.VerifiedOAuthIdentity{}, fmt.Errorf("Google OAuth response has no id_token")
	}
	payload, err := a.validate(ctx, rawIDToken, a.clientID)
	if err != nil || payload == nil || strings.TrimSpace(payload.Subject) == "" {
		return identityDomain.VerifiedOAuthIdentity{}, fmt.Errorf("validate Google ID token: %w", err)
	}
	nonce, _ := payload.Claims["nonce"].(string)
	if nonce != request.Nonce {
		return identityDomain.VerifiedOAuthIdentity{}, fmt.Errorf("Google ID token nonce mismatch")
	}
	verified := identityDomain.VerifiedOAuthIdentity{Provider: code, Subject: payload.Subject}
	if email, ok := payload.Claims["email"].(string); ok && strings.TrimSpace(email) != "" {
		email = strings.ToLower(strings.TrimSpace(email))
		verified.Email = &email
	}
	if emailVerified, ok := payload.Claims["email_verified"].(bool); ok {
		verified.EmailVerified = emailVerified
	}
	return verified, nil
}

func (a *Adapter) oauthConfig(redirectURI string) oauth2.Config {
	return oauth2.Config{
		ClientID:     a.clientID,
		ClientSecret: a.clientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     googleOAuth.Endpoint,
		Scopes:       []string{"openid", "email"},
	}
}

func (a *Adapter) allowsRedirectURI(redirectURI string) bool {
	_, ok := a.redirectURIs[strings.TrimSpace(redirectURI)]
	return ok
}

var _ identityDomain.OAuthProvider = (*Adapter)(nil)
