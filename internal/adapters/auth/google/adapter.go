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
		return nil, fmt.Errorf("google OAuth client id, client secret and allowed redirect URIs are required")
	}
	redirectURIs := make(map[string]struct{}, len(config.AllowedRedirectURIs))
	for _, redirectURI := range config.AllowedRedirectURIs {
		if redirectURI = strings.TrimSpace(redirectURI); redirectURI != "" {
			redirectURIs[redirectURI] = struct{}{}
		}
	}
	if len(redirectURIs) == 0 {
		return nil, fmt.Errorf("allowed redirect URIs are required for Google OAuth")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	return &Adapter{clientID: strings.TrimSpace(config.ClientID), clientSecret: strings.TrimSpace(config.ClientSecret), redirectURIs: redirectURIs, httpClient: config.HTTPClient, validate: googleIDToken.Validate}, nil
}

func (*Adapter) Code() string { return code }

func (a *Adapter) BeginAuthorization(_ context.Context, request identityDomain.OAuthAuthorizationRequest) (identityDomain.OAuthAuthorization, error) {
	if !a.allowsRedirectURI(request.RedirectURI) || strings.TrimSpace(request.State) == "" || strings.TrimSpace(request.Nonce) == "" || strings.TrimSpace(request.CodeVerifier) == "" {
		return identityDomain.OAuthAuthorization{}, fmt.Errorf("incomplete Google OAuth authorization request")
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
		return identityDomain.VerifiedOAuthIdentity{}, fmt.Errorf("incomplete Google OAuth code exchange")
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
		return identityDomain.VerifiedOAuthIdentity{}, fmt.Errorf("the Google OAuth response has no id_token")
	}
	payload, err := a.validate(ctx, rawIDToken, a.clientID)
	if err != nil || payload == nil || strings.TrimSpace(payload.Subject) == "" {
		return identityDomain.VerifiedOAuthIdentity{}, fmt.Errorf("validate Google ID token: %w", err)
	}
	nonce, _ := payload.Claims["nonce"].(string)
	if nonce != request.Nonce {
		return identityDomain.VerifiedOAuthIdentity{}, fmt.Errorf("the Google ID token nonce does not match")
	}
	verified := identityDomain.VerifiedOAuthIdentity{Provider: code, Subject: payload.Subject}
	if email, ok := payload.Claims["email"].(string); ok && strings.TrimSpace(email) != "" {
		email = strings.ToLower(strings.TrimSpace(email))
		verified.Email = &email
	}
	verifiedClaim, _ := payload.Claims["email_verified"].(bool)
	hostedDomain, _ := payload.Claims["hd"].(string)
	verified.EmailVerified = verified.Email != nil && ownsAddress(*verified.Email, verifiedClaim, hostedDomain)
	return verified, nil
}

// ownsAddress reports whether Google vouches for the account still holding this
// address, rather than only for it having been confirmed at some point.
//
// Google's own guidance is that email_verified alone does not prove current
// ownership of an address Google does not run: a custom-domain or Outlook
// address attached to a Google account can change hands while the Google
// account keeps it. It does prove ownership for an address in a domain Google
// runs — gmail.com — or in the Workspace domain the token names in hd, where
// the domain's administrator controls the mailbox.
//
// Everything else is still a usable sign-in: the account is created from it and
// the provider subject signs in from then on. It is only not treated as proof,
// so it never matches an existing local account or marks one verified.
func ownsAddress(email string, verifiedClaim bool, hostedDomain string) bool {
	if !verifiedClaim {
		return false
	}
	_, domain, ok := strings.Cut(strings.ToLower(email), "@")
	if !ok || domain == "" {
		return false
	}
	if domain == "gmail.com" || domain == "googlemail.com" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(hostedDomain), domain)
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
