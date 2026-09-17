package cloudflare

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

// SigningConfig carries the Stream signing material. Cloudflare returns both a
// key id and the private key when a signing key is created; the PEM form is
// taken here because Go parses it directly, while the equivalent JWK would
// have to be reassembled field by field.
type SigningConfig struct {
	// CustomerCode is the account's Stream subdomain, the value in
	// customer-<code>.cloudflarestream.com.
	CustomerCode string
	KeyID        string
	// PrivateKeyPEM is the base64-encoded PEM exactly as Cloudflare returns it
	// in result.pem. A raw PEM block is also accepted.
	PrivateKeyPEM string
}

type PlaybackSigner struct {
	customerCode string
	keyID        string
	privateKey   *rsa.PrivateKey
	now          func() time.Time
}

func NewPlaybackSigner(config SigningConfig) (*PlaybackSigner, error) {
	customerCode := strings.TrimSpace(config.CustomerCode)
	keyID := strings.TrimSpace(config.KeyID)
	if customerCode == "" || keyID == "" {
		return nil, fmt.Errorf("customer code and signing key ID are required for Cloudflare Stream")
	}
	// The subdomain is interpolated into a hostname, so anything that could
	// escape the label has to be refused rather than escaped.
	if strings.ContainsAny(customerCode, "/:?#@. ") {
		return nil, fmt.Errorf("invalid Cloudflare Stream customer code")
	}
	privateKey, err := parseSigningKey(config.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	return &PlaybackSigner{customerCode: customerCode, keyID: keyID, privateKey: privateKey, now: time.Now}, nil
}

// SignPlayback mints an RS256 token scoped to one asset and a deadline.
func (s *PlaybackSigner) SignPlayback(_ context.Context, externalID string, expiresAt time.Time) (video.Playback, error) {
	if s == nil || s.privateKey == nil {
		return video.Playback{}, video.ErrPlaybackUnavailable
	}
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return video.Playback{}, fmt.Errorf("%w: asset has no provider identifier", video.ErrPlaybackUnavailable)
	}
	now := s.now().UTC()
	if !expiresAt.After(now) {
		return video.Playback{}, fmt.Errorf("%w: expiry is not in the future", video.ErrPlaybackUnavailable)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		// Cloudflare reads the asset from sub and the signing key from kid.
		"sub": externalID,
		"kid": s.keyID,
		"exp": expiresAt.UTC().Unix(),
		"nbf": now.Add(-time.Minute).Unix(),
	})
	token.Header["kid"] = s.keyID
	signed, err := token.SignedString(s.privateKey)
	if err != nil {
		return video.Playback{}, fmt.Errorf("%w: %v", video.ErrPlaybackUnavailable, err)
	}

	// In a signed Stream URL the token takes the place the asset identifier
	// occupies in a public one, so the provider identifier never leaves us.
	base := "https://customer-" + s.customerCode + ".cloudflarestream.com/" + url.PathEscape(signed)
	return video.Playback{
		HLSURL:    base + "/manifest/video.m3u8",
		DASHURL:   base + "/manifest/video.mpd",
		ExpiresAt: expiresAt.UTC(),
	}, nil
}

// parseSigningKey accepts Cloudflare's base64-wrapped PEM or a plain PEM block,
// and both PKCS#1 and PKCS#8 encodings, because which one appears depends on
// how the key was exported.
func parseSigningKey(raw string) (*rsa.PrivateKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("a Cloudflare Stream signing key is required")
	}
	if !strings.Contains(raw, "-----BEGIN") {
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("the Cloudflare Stream signing key is neither PEM nor base64-encoded PEM")
		}
		raw = string(decoded)
	}
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, fmt.Errorf("the Cloudflare Stream signing key does not contain a PEM block")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("the Cloudflare Stream signing key is not a supported RSA private key")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("the Cloudflare Stream signing key must be RSA")
	}
	return key, nil
}

var _ video.PlaybackSigner = (*PlaybackSigner)(nil)
