package token

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Тестовий секретний ключ (>= 32 символи)
const testSecretKey = "this-is-a-test-secret-key-that-is-at-least-32-characters-long"

// ==========================================
// NewJWTMaker Tests
// ==========================================

func TestNewJWTMaker_Success(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)

	require.NoError(t, err)
	assert.NotNil(t, maker)
}

func TestNewJWTMaker_ShortKey(t *testing.T) {
	shortKeys := []string{
		"",
		"short",
		"1234567890",
		"0123456789012345678901234567890", // 31 символ — менше ніж 32
	}

	for _, key := range shortKeys {
		t.Run(key, func(t *testing.T) {
			maker, err := NewJWTMaker(key)

			assert.Error(t, err)
			assert.Nil(t, maker)
			assert.Contains(t, err.Error(), "invalid key size")
		})
	}
}

func TestNewJWTMaker_ExactMinLength(t *testing.T) {
	// Ключ рівно 32 символи
	key := "01234567890123456789012345678901"
	assert.Len(t, key, 32)

	maker, err := NewJWTMaker(key)

	require.NoError(t, err)
	assert.NotNil(t, maker)
}

func TestNewJWTMaker_ImplementsMakerInterface(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	// Перевіряємо, що повернутий об'єкт реалізує інтерфейс Maker
	_ = maker
}

// ==========================================
// CreateToken Tests
// ==========================================

func TestCreateToken_Success(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	userID := uuid.New()
	duration := 15 * time.Minute

	tokenStr, claims, err := maker.CreateToken(userID, 1, duration)

	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)
	assert.NotNil(t, claims)
	assert.Equal(t, "customer", claims.Role)
}

func TestCreateTokenForRoleStoresStringRole(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)
	tokenString, _, err := maker.CreateTokenForRole(uuid.New(), RoleAdmin, time.Minute)
	require.NoError(t, err)
	claims, err := maker.VerifyToken(tokenString)
	require.NoError(t, err)
	assert.Equal(t, RoleAdmin, claims.Role)
	assert.Zero(t, claims.RoleID, "legacy integer role must not be serialized")
}

func TestCreateToken_CorrectClaims(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	userID := uuid.New()
	duration := 1 * time.Hour

	_, claims, err := maker.CreateToken(userID, 1, duration)

	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID, "UserID в claims має збігатися")
	assert.NotEmpty(t, claims.ID, "JWT ID має бути заповненим")
	assert.NotNil(t, claims.IssuedAt, "IssuedAt має бути встановленим")
	assert.NotNil(t, claims.ExpiresAt, "ExpiresAt має бути встановленим")
}

func TestCreateToken_ExpiresAtCorrectTime(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	userID := uuid.New()
	duration := 30 * time.Minute

	beforeCreate := time.Now()
	_, claims, err := maker.CreateToken(userID, 1, duration)
	afterCreate := time.Now()

	require.NoError(t, err)

	// Перевіряємо, що IssuedAt в очікуваних межах
	assert.True(t, !claims.IssuedAt.Before(beforeCreate.Add(-time.Second)),
		"IssuedAt не повинен бути до часу створення")
	assert.True(t, !claims.IssuedAt.After(afterCreate.Add(time.Second)),
		"IssuedAt не повинен бути після часу створення")

	// Перевіряємо, що ExpiresAt відповідає duration
	expectedExpiry := claims.IssuedAt.Add(duration)
	assert.WithinDuration(t, expectedExpiry, claims.ExpiresAt.Time, time.Second,
		"ExpiresAt має відповідати IssuedAt + duration")
}

func TestCreateToken_UniqueJTI(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	userID := uuid.New()
	duration := 15 * time.Minute

	_, claims1, err := maker.CreateToken(userID, 1, duration)
	require.NoError(t, err)

	_, claims2, err := maker.CreateToken(userID, 1, duration)
	require.NoError(t, err)

	assert.NotEqual(t, claims1.ID, claims2.ID,
		"Кожен токен має мати унікальний JWT ID (jti)")
}

func TestCreateToken_UniqueTokenStrings(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	userID := uuid.New()
	duration := 15 * time.Minute

	token1, _, err := maker.CreateToken(userID, 1, duration)
	require.NoError(t, err)

	token2, _, err := maker.CreateToken(userID, 1, duration)
	require.NoError(t, err)

	assert.NotEqual(t, token1, token2,
		"Два різних виклики CreateToken мають генерувати різні рядки токенів")
}

func TestCreateToken_DifferentDurations(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	userID := uuid.New()

	durations := []time.Duration{
		1 * time.Second,
		5 * time.Minute,
		1 * time.Hour,
		24 * time.Hour,
		7 * 24 * time.Hour,
	}

	for _, d := range durations {
		t.Run(d.String(), func(t *testing.T) {
			tokenStr, claims, err := maker.CreateToken(userID, 1, d)

			require.NoError(t, err)
			assert.NotEmpty(t, tokenStr)
			assert.WithinDuration(t, claims.IssuedAt.Add(d), claims.ExpiresAt.Time, time.Second)
		})
	}
}

// ==========================================
// VerifyToken Tests
// ==========================================

func TestVerifyToken_ValidToken(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	userID := uuid.New()
	duration := 15 * time.Minute

	tokenStr, originalClaims, err := maker.CreateToken(userID, 1, duration)
	require.NoError(t, err)

	// Перевіряємо токен
	verifiedClaims, err := maker.VerifyToken(tokenStr)

	require.NoError(t, err)
	assert.Equal(t, originalClaims.UserID, verifiedClaims.UserID)
	assert.Equal(t, originalClaims.ID, verifiedClaims.ID)
	assert.WithinDuration(t, originalClaims.IssuedAt.Time, verifiedClaims.IssuedAt.Time, time.Second)
	assert.WithinDuration(t, originalClaims.ExpiresAt.Time, verifiedClaims.ExpiresAt.Time, time.Second)
}

func TestVerifyToken_ExpiredToken(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	userID := uuid.New()
	// Створюємо токен з від'ємним терміном дії — він одразу прострочений
	duration := -1 * time.Minute

	tokenStr, _, err := maker.CreateToken(userID, 1, duration)
	require.NoError(t, err)

	// Прострочений токен має бути відхилений
	claims, err := maker.VerifyToken(tokenStr)

	assert.Error(t, err)
	assert.Nil(t, claims)
	assert.Contains(t, err.Error(), "invalid token")
}

func TestVerifyToken_InvalidSignature(t *testing.T) {
	// Створюємо токен з одним ключем
	maker1, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	// Створюємо другий maker з іншим ключем
	differentKey := "another-secret-key-that-is-at-least-32-characters-long-!!!"
	maker2, err := NewJWTMaker(differentKey)
	require.NoError(t, err)

	userID := uuid.New()
	tokenStr, _, err := maker1.CreateToken(userID, 1, 15*time.Minute)
	require.NoError(t, err)

	// Перевіряємо токен з іншим ключем — підпис не збігається
	claims, err := maker2.VerifyToken(tokenStr)

	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestVerifyToken_MalformedToken(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	malformedTokens := []string{
		"",
		"not-a-jwt",
		"header.payload.signature.extra",
		"eyJhbGciOiJIUzI1NiJ9.invalid.payload",
		"abc.def.ghi",
	}

	for _, tok := range malformedTokens {
		t.Run(tok, func(t *testing.T) {
			claims, err := maker.VerifyToken(tok)

			assert.Error(t, err)
			assert.Nil(t, claims)
		})
	}
}

func TestVerifyToken_TamperedPayload(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	userID := uuid.New()
	tokenStr, _, err := maker.CreateToken(userID, 1, 15*time.Minute)
	require.NoError(t, err)

	// Модифікуємо один символ у payload частині токена
	tokenBytes := []byte(tokenStr)
	// JWT формат: header.payload.signature — змінимо символ у payload
	dotCount := 0
	for i, b := range tokenBytes {
		if b == '.' {
			dotCount++
		}
		if dotCount == 1 && b != '.' {
			// Змінимо байт у payload
			if tokenBytes[i] == 'A' {
				tokenBytes[i] = 'B'
			} else {
				tokenBytes[i] = 'A'
			}
			break
		}
	}

	tamperedToken := string(tokenBytes)
	claims, err := maker.VerifyToken(tamperedToken)

	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestVerifyToken_WrongSigningMethod(t *testing.T) {
	// Створюємо токен з алгоритмом "none" — має бути відхилений
	userID := uuid.New()
	claims := &CustomClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}

	// Підписуємо токен методом "none" (алгоритм без підпису)
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenStr, err := jwtToken.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	// JWTMaker повинен відхилити токен з невірним алгоритмом
	verifiedClaims, err := maker.VerifyToken(tokenStr)

	assert.Error(t, err)
	assert.Nil(t, verifiedClaims)
}

// ==========================================
// CreateToken + VerifyToken Round-Trip
// ==========================================

func TestCreateAndVerify_RoundTrip(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	userID := uuid.New()
	duration := 1 * time.Hour

	tokenStr, originalClaims, err := maker.CreateToken(userID, 1, duration)
	require.NoError(t, err)

	verifiedClaims, err := maker.VerifyToken(tokenStr)
	require.NoError(t, err)

	assert.Equal(t, originalClaims.UserID, verifiedClaims.UserID)
	assert.Equal(t, originalClaims.ID, verifiedClaims.ID)
}

func TestCreateAndVerify_MultipleUsers(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	users := []uuid.UUID{
		uuid.New(),
		uuid.New(),
		uuid.New(),
	}

	tokens := make([]string, len(users))
	for i, userID := range users {
		tokenStr, _, err := maker.CreateToken(userID, 1, 1*time.Hour)
		require.NoError(t, err)
		tokens[i] = tokenStr
	}

	// Перевіряємо кожен токен і переконуємося, що UserID збігається
	for i, tokenStr := range tokens {
		claims, err := maker.VerifyToken(tokenStr)
		require.NoError(t, err)
		assert.Equal(t, users[i], claims.UserID,
			"Токен %d має містити правильний UserID", i)
	}
}

func TestVerifyTokenRejectsTokenMintedForAnotherPurpose(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	// Same key, same issuer and audience, but not an API access token. Without
	// the typ claim a refresh or single-use token would authenticate requests.
	claims := &CustomClaims{
		UserID: uuid.New(), Role: RoleCustomer, TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ID: uuid.NewString(), Issuer: DefaultIssuer,
			Audience:  jwt.ClaimStrings{DefaultAudience},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecretKey))
	require.NoError(t, err)

	verified, err := maker.VerifyToken(signed)
	assert.Nil(t, verified)
	assert.ErrorContains(t, err, "invalid token type")
}

func TestVerifyTokenRejectsTokenFromAnotherDeployment(t *testing.T) {
	// Two stores may legitimately share JWT_SECRET; the configuration permits
	// it. Distinct issuer and audience keep their sessions separate.
	storeA, err := NewJWTMakerFor(testSecretKey, "store-a", "store-a-api")
	require.NoError(t, err)
	storeB, err := NewJWTMakerFor(testSecretKey, "store-b", "store-b-api")
	require.NoError(t, err)

	signed, _, err := storeA.CreateTokenForRole(uuid.New(), RoleAdmin, time.Hour)
	require.NoError(t, err)

	accepted, err := storeA.VerifyToken(signed)
	require.NoError(t, err)
	assert.Equal(t, RoleAdmin, accepted.Role)

	rejected, err := storeB.VerifyToken(signed)
	assert.Nil(t, rejected)
	assert.Error(t, err)
}

func TestIdentityForStoreSeparatesTwoStoresSharingASecret(t *testing.T) {
	// This is the production path: cmd/api derives the identity from
	// STORE_CODE rather than naming an issuer by hand, so the derivation
	// itself has to be what keeps two deployments apart.
	issuerA, audienceA := IdentityForStore("northwind")
	issuerB, audienceB := IdentityForStore("contoso")
	require.NotEqual(t, issuerA, issuerB)
	require.NotEqual(t, audienceA, audienceB)

	storeA, err := NewJWTMakerFor(testSecretKey, issuerA, audienceA)
	require.NoError(t, err)
	storeB, err := NewJWTMakerFor(testSecretKey, issuerB, audienceB)
	require.NoError(t, err)

	signed, _, err := storeA.CreateTokenForRole(uuid.New(), RoleAdmin, time.Hour)
	require.NoError(t, err)

	verified, err := storeA.VerifyToken(signed)
	require.NoError(t, err)
	assert.Equal(t, RoleAdmin, verified.Role)

	rejected, err := storeB.VerifyToken(signed)
	assert.Nil(t, rejected)
	assert.Error(t, err)

	// A deployment that still uses the shared defaults must not be a way back
	// in either, or the separation would only hold between configured stores.
	shared, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)
	rejected, err = shared.VerifyToken(signed)
	assert.Nil(t, rejected)
	assert.Error(t, err)
}

func TestIdentityForStoreIsNormalizedAndFallsBackWithoutAStore(t *testing.T) {
	// STORE_CODE is already validated as lowercase, but tooling calls this
	// with whatever it has; a differently cased code must not produce a second
	// identity for the same store.
	issuer, audience := IdentityForStore("  Northwind  ")
	expectedIssuer, expectedAudience := IdentityForStore("northwind")
	assert.Equal(t, expectedIssuer, issuer)
	assert.Equal(t, expectedAudience, audience)

	// No store configured: degrade to the previous behaviour rather than mint
	// tokens with a trailing separator that nothing would verify.
	issuer, audience = IdentityForStore("")
	assert.Equal(t, DefaultIssuer, issuer)
	assert.Equal(t, DefaultAudience, audience)
}

func TestVerifyTokenRejectsUnexpectedSigningAlgorithm(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	// "none" carries no signature at all; it must be refused by the algorithm
	// allow-list rather than reaching signature verification.
	claims := &CustomClaims{
		UserID: uuid.New(), Role: RoleAdmin, TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: DefaultIssuer, Audience: jwt.ClaimStrings{DefaultAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	verified, err := maker.VerifyToken(unsigned)
	assert.Nil(t, verified)
	assert.Error(t, err)
}

func TestVerifyTokenRejectsUnknownRole(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	claims := &CustomClaims{
		UserID: uuid.New(), Role: "superuser", TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: DefaultIssuer, Audience: jwt.ClaimStrings{DefaultAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecretKey))
	require.NoError(t, err)

	verified, err := maker.VerifyToken(signed)
	assert.Nil(t, verified)
	assert.ErrorContains(t, err, "invalid token role")
}

func TestCreateTokenStampsPurposeIssuerAndAudience(t *testing.T) {
	maker, err := NewJWTMaker(testSecretKey)
	require.NoError(t, err)

	signed, claims, err := maker.CreateTokenForRole(uuid.New(), RoleManager, time.Hour)
	require.NoError(t, err)
	assert.Equal(t, TokenTypeAccess, claims.TokenType)
	assert.Equal(t, DefaultIssuer, claims.Issuer)
	assert.Equal(t, jwt.ClaimStrings{DefaultAudience}, claims.Audience)

	verified, err := maker.VerifyToken(signed)
	require.NoError(t, err)
	assert.Equal(t, TokenTypeAccess, verified.TokenType)
}
