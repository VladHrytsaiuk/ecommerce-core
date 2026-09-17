package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidUnsubscribeToken covers every way a token can fail: wrong shape,
// wrong signature, or expired. They are deliberately indistinguishable to the
// caller — telling someone which one it was turns the endpoint into an oracle.
var ErrInvalidUnsubscribeToken = errors.New("invalid unsubscribe token")

// UnsubscribeSigner mints and checks the token that lets a guest withdraw
// marketing consent from a link in an email.
//
// A guest has no account, so the only ways to offer this are an authenticated
// session they do not have, or a capability they can carry. The service could
// already withdraw consent by email address and said in a comment that it
// "supports a signed unsubscribe endpoint" — but nothing signed anything and no
// endpoint existed, so a guest who received an abandoned-cart email had no
// implemented way to stop receiving them.
//
// The token carries the address and an expiry and is signed with a key derived
// from the application secret for this purpose alone, so it cannot be replayed
// against anything else that uses the same secret.
type UnsubscribeSigner struct {
	key []byte
	ttl time.Duration
}

const unsubscribeKeyPurpose = "consent:unsubscribe:v1"

func NewUnsubscribeSigner(secret string, ttl time.Duration) (*UnsubscribeSigner, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("unsubscribe signer requires a secret")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("unsubscribe token lifetime must be positive")
	}
	// Derived, not the secret itself: a token signed here must not be a valid
	// anything anywhere else that signs with the same configured secret.
	derived := hmac.New(sha256.New, []byte(secret))
	derived.Write([]byte(unsubscribeKeyPurpose))
	return &UnsubscribeSigner{key: derived.Sum(nil), ttl: ttl}, nil
}

// Sign returns a token for one address, valid for the configured lifetime.
func (s *UnsubscribeSigner) Sign(email string, now time.Time) (string, error) {
	email = normalizeUnsubscribeEmail(email)
	if email == "" {
		return "", ErrInvalid
	}
	payload := email + "|" + strconv.FormatInt(now.UTC().Add(s.ttl).Unix(), 10)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(s.sign(encoded)), nil
}

// Verify returns the address a valid, unexpired token was issued for.
func (s *UnsubscribeSigner) Verify(token string, now time.Time) (string, error) {
	encoded, signature, found := strings.Cut(strings.TrimSpace(token), ".")
	if !found || encoded == "" || signature == "" {
		return "", ErrInvalidUnsubscribeToken
	}
	provided, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return "", ErrInvalidUnsubscribeToken
	}
	// Constant time, so the endpoint cannot be used to search for a signature.
	if !hmac.Equal(provided, s.sign(encoded)) {
		return "", ErrInvalidUnsubscribeToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrInvalidUnsubscribeToken
	}
	email, expiry, found := strings.Cut(string(payload), "|")
	if !found {
		return "", ErrInvalidUnsubscribeToken
	}
	seconds, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil || now.UTC().After(time.Unix(seconds, 0).UTC()) {
		return "", ErrInvalidUnsubscribeToken
	}
	if email = normalizeUnsubscribeEmail(email); email == "" {
		return "", ErrInvalidUnsubscribeToken
	}
	return email, nil
}

func (s *UnsubscribeSigner) sign(encoded string) []byte {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(encoded))
	return mac.Sum(nil)
}

func normalizeUnsubscribeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
