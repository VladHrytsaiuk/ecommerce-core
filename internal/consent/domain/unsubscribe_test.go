package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// This token is the only thing standing between a link in an email and the
// ability to unsubscribe an arbitrary address, so it is worth stating what it
// does and does not allow.

func TestATokenRoundTripsToTheAddressItWasIssuedFor(t *testing.T) {
	signer := mustSigner(t, "application-secret", time.Hour)
	now := time.Now().UTC()

	token, err := signer.Sign("  Guest@Example.TEST ", now)
	if err != nil {
		t.Fatal(err)
	}
	email, err := signer.Verify(token, now)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if email != "guest@example.test" {
		t.Fatalf("Verify() = %q, want the normalised address", email)
	}
}

func TestATamperedTokenIsRefused(t *testing.T) {
	// Without the signature check, anyone could unsubscribe anyone by editing
	// the address in the link.
	signer := mustSigner(t, "application-secret", time.Hour)
	now := time.Now().UTC()
	token, err := signer.Sign("victim@example.test", now)
	if err != nil {
		t.Fatal(err)
	}
	payload, signature, _ := strings.Cut(token, ".")

	forged, err := signer.Sign("attacker@example.test", now)
	if err != nil {
		t.Fatal(err)
	}
	forgedPayload, _, _ := strings.Cut(forged, ".")

	for name, candidate := range map[string]string{
		"swapped payload":   forgedPayload + "." + signature,
		"swapped signature": payload + "." + strings.Repeat("A", len(signature)),
		"no separator":      payload + signature,
		"empty":             "",
		"payload only":      payload,
		"not base64":        "!!!.???",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := signer.Verify(candidate, now); !errors.Is(err, ErrInvalidUnsubscribeToken) {
				t.Fatalf("Verify(%q) error = %v, want refusal", name, err)
			}
		})
	}
}

func TestATokenFromAnotherSecretIsRefused(t *testing.T) {
	now := time.Now().UTC()
	token, err := mustSigner(t, "one-secret", time.Hour).Sign("guest@example.test", now)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := mustSigner(t, "another-secret", time.Hour).Verify(token, now); !errors.Is(err, ErrInvalidUnsubscribeToken) {
		t.Fatalf("Verify() error = %v, want refusal", err)
	}
}

func TestATokenExpires(t *testing.T) {
	// A link in an email outlives the mailbox it sits in. An unbounded
	// capability is one forwarded message away from being someone else's.
	signer := mustSigner(t, "application-secret", time.Hour)
	issued := time.Now().UTC()
	token, err := signer.Sign("guest@example.test", issued)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := signer.Verify(token, issued.Add(59*time.Minute)); err != nil {
		t.Fatalf("Verify() inside the window error = %v", err)
	}
	if _, err := signer.Verify(token, issued.Add(time.Hour+time.Second)); !errors.Is(err, ErrInvalidUnsubscribeToken) {
		t.Fatalf("Verify() after expiry error = %v, want refusal", err)
	}
}

func TestTheSigningKeyIsScopedToThisPurpose(t *testing.T) {
	// The same application secret signs other things. A token minted here must
	// not be a valid anything elsewhere, and the derivation is what ensures it.
	signer := mustSigner(t, "application-secret", time.Hour)
	if string(signer.key) == "application-secret" {
		t.Fatal("the raw application secret is used as the signing key")
	}
	other, err := NewUnsubscribeSigner("application-secret", 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if string(signer.key) != string(other.key) {
		t.Fatal("the key depends on the lifetime; a configuration change would invalidate every live link")
	}
}

func TestASignerIsRefusedWithoutASecretOrALifetime(t *testing.T) {
	if _, err := NewUnsubscribeSigner("  ", time.Hour); err == nil {
		t.Fatal("NewUnsubscribeSigner() accepted a blank secret")
	}
	if _, err := NewUnsubscribeSigner("secret", 0); err == nil {
		t.Fatal("NewUnsubscribeSigner() accepted a token that never expires")
	}
}

func TestABlankAddressIsNotSigned(t *testing.T) {
	if _, err := mustSigner(t, "secret", time.Hour).Sign("   ", time.Now()); !errors.Is(err, ErrInvalid) {
		t.Fatal("Sign() minted a token for no address")
	}
}

func mustSigner(t *testing.T, secret string, ttl time.Duration) *UnsubscribeSigner {
	t.Helper()
	signer, err := NewUnsubscribeSigner(secret, ttl)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}
