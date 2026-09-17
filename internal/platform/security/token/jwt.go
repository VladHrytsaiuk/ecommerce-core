package token

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// CustomClaims is the payload carried by an issued JWT.
type CustomClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role"`
	// TokenType separates an API access token from any other token this key
	// might sign later. Without it a refresh or single-use token issued under
	// the same secret would authenticate an API request just as well.
	TokenType string `json:"typ"`
	// RoleID is retained only for legacy in-process callers. It is deliberately
	// not serialized into new JWTs; authorization must use the stable role code.
	RoleID int `json:"-"`
	jwt.RegisteredClaims
}

const (
	// TokenTypeAccess is the only purpose VerifyToken accepts.
	TokenTypeAccess = "access"
	// DefaultIssuer and DefaultAudience are the fallback identity, used only
	// where no store is known — tests and tooling. They are the same constants
	// in every build, so they bind a token to the software, not to a
	// deployment: two stores using them both accept each other's tokens.
	// Production entrypoints pass a store identity instead; see
	// IdentityForStore.
	DefaultIssuer   = "ecommerce-core"
	DefaultAudience = "ecommerce-core-api"
)

// IdentityForStore derives the JWT issuer and audience for one deployment.
//
// This core is copied per store, and nothing stops two of those copies from
// being configured with the same JWT_SECRET — a shared secrets manager, a
// staging environment cloned from production, a template .env that was never
// changed. The secret alone therefore cannot separate them. STORE_CODE can:
// it is required, validated, and unique to the deployment by definition.
//
// An empty code returns the defaults rather than a malformed identity, so a
// caller without a store configuration degrades to the previous behaviour
// instead of minting tokens nothing can verify.
func IdentityForStore(storeCode string) (issuer, audience string) {
	code := strings.ToLower(strings.TrimSpace(storeCode))
	if code == "" {
		return DefaultIssuer, DefaultAudience
	}
	return DefaultIssuer + "/" + code, DefaultAudience + "/" + code
}

const (
	RoleCustomer = "customer"
	RoleManager  = "manager"
	RoleAdmin    = "admin"
	RoleOwner    = "owner"
)

// Maker is the port for issuing and verifying tokens.
type Maker interface {
	// CreateTokenForRole is the clean token API. Role codes mirror the Core
	// users.role vocabulary and are the only values authorization consumes.
	CreateTokenForRole(userID uuid.UUID, role string, duration time.Duration) (string, *CustomClaims, error)
	// CreateToken remains only for legacy callers with integer role IDs.
	CreateToken(userID uuid.UUID, roleID int, duration time.Duration) (string, *CustomClaims, error)
	VerifyToken(token string) (*CustomClaims, error)
}

// JWTMaker implements Maker with JSON Web Tokens.
type JWTMaker struct {
	secretKey string
	issuer    string
	audience  string
}

// NewJWTMaker builds a token maker over the configured signing secret.
func NewJWTMaker(secretKey string) (Maker, error) {
	// A short HMAC secret is brute-forceable offline once a token leaks.
	if len(secretKey) < 32 {
		return nil, fmt.Errorf("invalid key size: must be at least 32 characters")
	}
	return &JWTMaker{secretKey: secretKey, issuer: DefaultIssuer, audience: DefaultAudience}, nil
}

// NewJWTMakerFor scopes tokens to one deployment. Two stores sharing a secret
// but using different identities can no longer accept each other's tokens.
func NewJWTMakerFor(secretKey, issuer, audience string) (Maker, error) {
	maker, err := NewJWTMaker(secretKey)
	if err != nil {
		return nil, err
	}
	concrete := maker.(*JWTMaker)
	if trimmed := strings.TrimSpace(issuer); trimmed != "" {
		concrete.issuer = trimmed
	}
	if trimmed := strings.TrimSpace(audience); trimmed != "" {
		concrete.audience = trimmed
	}
	return concrete, nil
}

// CreateToken issues a token for one subject, role and lifetime.
func (maker *JWTMaker) CreateToken(userID uuid.UUID, roleID int, duration time.Duration) (string, *CustomClaims, error) {
	return maker.CreateTokenForRole(userID, roleCode(roleID), duration)
}

func (maker *JWTMaker) CreateTokenForRole(userID uuid.UUID, role string, duration time.Duration) (string, *CustomClaims, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	if !validRole(role) {
		return "", nil, fmt.Errorf("invalid role %q", role)
	}
	now := time.Now()
	claims := &CustomClaims{
		UserID:    userID,
		Role:      role,
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(), // Unique per token, so an issued token can be told apart from a replay.
			Issuer:    maker.issuer,
			Audience:  jwt.ClaimStrings{maker.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
		},
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := jwtToken.SignedString([]byte(maker.secretKey))
	if err != nil {
		return "", nil, fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, claims, nil
}

func roleCode(roleID int) string {
	switch roleID {
	case 1:
		return "customer"
	case 2:
		return "admin"
	case 3:
		return "owner"
	default:
		return ""
	}
}

func validRole(role string) bool {
	switch role {
	case RoleCustomer, RoleManager, RoleAdmin, RoleOwner:
		return true
	default:
		return false
	}
}

// VerifyToken checks the signature and the token's validity window.
func (maker *JWTMaker) VerifyToken(token string) (*CustomClaims, error) {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		// HMAC only: accepting the algorithm named in the header is how a
		// token signed with "none" — or with the public key — gets accepted.
		_, ok := token.Method.(*jwt.SigningMethodHMAC)
		if !ok {
			return nil, errors.New("invalid token signing method")
		}
		return []byte(maker.secretKey), nil
	}

	jwtToken, err := jwt.ParseWithClaims(token, &CustomClaims{}, keyFunc,
		// Reject the algorithm before the signature is even considered, so a
		// token cannot be re-presented under a method the parser would accept.
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(maker.issuer),
		jwt.WithAudience(maker.audience),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := jwtToken.Claims.(*CustomClaims)
	if !ok || !jwtToken.Valid {
		return nil, errors.New("invalid token claims")
	}
	// A token minted for any other purpose must never authenticate a request,
	// even though it carries a valid signature from the same key.
	if claims.TokenType != TokenTypeAccess {
		return nil, errors.New("invalid token type")
	}
	if !validRole(claims.Role) {
		return nil, errors.New("invalid token role")
	}

	return claims, nil
}
