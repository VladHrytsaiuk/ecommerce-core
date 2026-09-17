package cloudflare

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

func TestSignPlaybackMintsAVerifiableTokenScopedToOneAsset(t *testing.T) {
	key := testSigningKey(t)
	signer := newTestSigner(t, SigningConfig{CustomerCode: "abc123", KeyID: "key-1", PrivateKeyPEM: pkcs1PEM(t, key)})

	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	grant, err := signer.SignPlayback(context.Background(), "stream-uid", expiresAt)
	if err != nil {
		t.Fatalf("SignPlayback() error = %v", err)
	}
	if !grant.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("grant expiry = %s, want %s", grant.ExpiresAt, expiresAt)
	}

	token := tokenFromURL(t, grant.HLSURL)
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) { return &key.PublicKey, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}))
	if err != nil || !parsed.Valid {
		t.Fatalf("token did not verify against the public key: %v", err)
	}
	// Cloudflare reads the asset from sub and the signing key from kid; a token
	// missing either would be accepted here but rejected by the provider.
	if claims["sub"] != "stream-uid" || claims["kid"] != "key-1" {
		t.Fatalf("claims = %v, want sub=stream-uid and kid=key-1", claims)
	}
	if parsed.Header["kid"] != "key-1" {
		t.Fatalf("header kid = %v, want key-1", parsed.Header["kid"])
	}
}

func TestSignPlaybackProducesHLSAndDASHManifestsOnTheCustomerSubdomain(t *testing.T) {
	signer := newTestSigner(t, SigningConfig{CustomerCode: "abc123", KeyID: "key-1", PrivateKeyPEM: pkcs1PEM(t, testSigningKey(t))})

	grant, err := signer.SignPlayback(context.Background(), "stream-uid", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("SignPlayback() error = %v", err)
	}
	for _, check := range []struct{ url, suffix string }{
		{grant.HLSURL, "/manifest/video.m3u8"},
		{grant.DASHURL, "/manifest/video.mpd"},
	} {
		if !strings.HasPrefix(check.url, "https://customer-abc123.cloudflarestream.com/") || !strings.HasSuffix(check.url, check.suffix) {
			t.Fatalf("playback URL = %s, want the customer subdomain and %s", check.url, check.suffix)
		}
	}
	// A public Stream URL carries the asset UID in this position. Leaking it
	// would let a client address the asset directly forever.
	if strings.Contains(grant.HLSURL, "/stream-uid/") {
		t.Fatalf("playback URL exposes the provider asset identifier: %s", grant.HLSURL)
	}
}

func TestSignPlaybackRefusesAnExpiryThatIsNotInTheFuture(t *testing.T) {
	signer := newTestSigner(t, SigningConfig{CustomerCode: "abc123", KeyID: "key-1", PrivateKeyPEM: pkcs1PEM(t, testSigningKey(t))})

	// An already-expired grant would be handed to a visitor as a broken player
	// rather than an error anyone could act on.
	for _, expiry := range []time.Time{time.Now().Add(-time.Minute), time.Now().Add(-time.Hour)} {
		if _, err := signer.SignPlayback(context.Background(), "stream-uid", expiry); err == nil {
			t.Fatalf("SignPlayback(%s) error = nil, want refusal", expiry)
		}
	}
}

func TestSignPlaybackRefusesAnAssetWithNoProviderIdentifier(t *testing.T) {
	signer := newTestSigner(t, SigningConfig{CustomerCode: "abc123", KeyID: "key-1", PrivateKeyPEM: pkcs1PEM(t, testSigningKey(t))})

	if _, err := signer.SignPlayback(context.Background(), "  ", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("SignPlayback() error = nil, want refusal for an asset that never reached the provider")
	}
}

func TestNewPlaybackSignerAcceptsBothPEMEncodings(t *testing.T) {
	key := testSigningKey(t)
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8 := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes}))

	// Which encoding appears depends on how the key was exported, and
	// Cloudflare additionally wraps it in base64.
	for name, encoded := range map[string]string{
		"pkcs1":           pkcs1PEM(t, key),
		"pkcs8":           pkcs8,
		"base64 of pkcs1": base64.StdEncoding.EncodeToString([]byte(pkcs1PEM(t, key))),
		"base64 of pkcs8": base64.StdEncoding.EncodeToString([]byte(pkcs8)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewPlaybackSigner(SigningConfig{CustomerCode: "abc123", KeyID: "key-1", PrivateKeyPEM: encoded}); err != nil {
				t.Fatalf("NewPlaybackSigner() error = %v", err)
			}
		})
	}
}

func TestNewPlaybackSignerRejectsUnusableConfiguration(t *testing.T) {
	valid := pkcs1PEM(t, testSigningKey(t))
	for name, config := range map[string]SigningConfig{
		"missing customer code": {KeyID: "key-1", PrivateKeyPEM: valid},
		"missing key id":        {CustomerCode: "abc123", PrivateKeyPEM: valid},
		"missing key":           {CustomerCode: "abc123", KeyID: "key-1"},
		"not a key":             {CustomerCode: "abc123", KeyID: "key-1", PrivateKeyPEM: "not-a-key"},
		// The code is interpolated into a hostname, so a value that could
		// escape the label must be refused rather than escaped.
		"customer code escapes the host": {CustomerCode: "abc/../evil.test", KeyID: "key-1", PrivateKeyPEM: valid},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewPlaybackSigner(config); err == nil {
				t.Fatal("NewPlaybackSigner() error = nil, want refusal")
			}
		})
	}
}

func newTestSigner(t *testing.T, config SigningConfig) *PlaybackSigner {
	t.Helper()
	signer, err := NewPlaybackSigner(config)
	if err != nil {
		t.Fatalf("NewPlaybackSigner() error = %v", err)
	}
	return signer
}

func testSigningKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func pkcs1PEM(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

// tokenFromURL extracts the path segment a signed Stream URL puts the token in.
func tokenFromURL(t *testing.T, playbackURL string) string {
	t.Helper()
	const host = "https://customer-abc123.cloudflarestream.com/"
	trimmed := strings.TrimPrefix(playbackURL, host)
	token, _, found := strings.Cut(trimmed, "/manifest/")
	if !found {
		t.Fatalf("playback URL %q has no token segment", playbackURL)
	}
	return token
}

var _ video.PlaybackSigner = (*PlaybackSigner)(nil)
