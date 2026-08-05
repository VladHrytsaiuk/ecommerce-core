package token

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// CustomClaims містить корисне навантаження для JWT.
type CustomClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role"`
	// RoleID is retained only for legacy in-process callers. It is deliberately
	// not serialized into new JWTs; authorization must use the stable role code.
	RoleID int `json:"-"`
	jwt.RegisteredClaims
}

const (
	RoleCustomer = "customer"
	RoleManager  = "manager"
	RoleAdmin    = "admin"
	RoleOwner    = "owner"
)

// Maker визначає інтерфейс для створення та перевірки токенів.
type Maker interface {
	// CreateTokenForRole is the clean token API. Role codes mirror the Core
	// users.role vocabulary and are the only values authorization consumes.
	CreateTokenForRole(userID uuid.UUID, role string, duration time.Duration) (string, *CustomClaims, error)
	// CreateToken remains only for legacy callers with integer role IDs.
	CreateToken(userID uuid.UUID, roleID int, duration time.Duration) (string, *CustomClaims, error)
	VerifyToken(token string) (*CustomClaims, error)
}

// JWTMaker - реалізація до інтерфейсу Maker за допомогою JWT.
type JWTMaker struct {
	secretKey string
}

// NewJWTMaker створює нового JWTMaker для роботи з токенами.
func NewJWTMaker(secretKey string) (Maker, error) {
	// Базова перевірка довжини ключа
	if len(secretKey) < 32 {
		return nil, fmt.Errorf("invalid key size: must be at least 32 characters")
	}
	return &JWTMaker{secretKey}, nil
}

// CreateToken генерує новий JWT токен для заданого userID, ролі та терміну.
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
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(), // JWT ID, унікальний для кожного токена (практично запобігає reuse-атакам)
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

// VerifyToken перевіряє криптографічний підпис токена та його життєздатність.
func (maker *JWTMaker) VerifyToken(token string) (*CustomClaims, error) {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		// Очікуваний алгоритм — HMAC (HS256)
		_, ok := token.Method.(*jwt.SigningMethodHMAC)
		if !ok {
			return nil, errors.New("invalid token signing method")
		}
		return []byte(maker.secretKey), nil
	}

	jwtToken, err := jwt.ParseWithClaims(token, &CustomClaims{}, keyFunc)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := jwtToken.Claims.(*CustomClaims)
	if !ok || !jwtToken.Valid {
		return nil, errors.New("invalid token claims")
	}

	return claims, nil
}
