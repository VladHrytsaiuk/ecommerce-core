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
	var _ Maker = maker
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
	assert.True(t, !claims.IssuedAt.Time.Before(beforeCreate.Add(-time.Second)),
		"IssuedAt не повинен бути до часу створення")
	assert.True(t, !claims.IssuedAt.Time.After(afterCreate.Add(time.Second)),
		"IssuedAt не повинен бути після часу створення")

	// Перевіряємо, що ExpiresAt відповідає duration
	expectedExpiry := claims.IssuedAt.Time.Add(duration)
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
			assert.WithinDuration(t, claims.IssuedAt.Time.Add(d), claims.ExpiresAt.Time, time.Second)
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
