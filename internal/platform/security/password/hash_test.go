package password

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// ==========================================
// HashPassword Tests
// ==========================================

func TestHashPassword_Success(t *testing.T) {
	password := "SecureP@ssw0rd!"

	hashedPassword, err := HashPassword(password)

	require.NoError(t, err)
	assert.NotEmpty(t, hashedPassword)
	assert.NotEqual(t, password, hashedPassword, "Хеш має відрізнятися від оригінального пароля")
}

func TestHashPassword_ProducesBcryptHash(t *testing.T) {
	password := "MyPassword123"

	hashedPassword, err := HashPassword(password)

	require.NoError(t, err)

	// Перевіряємо, що результат є валідним bcrypt хешем
	// bcrypt хеші завжди починаються з "$2a$" або "$2b$"
	assert.Regexp(t, `^\$2[aby]?\$`, hashedPassword, "Має бути валідний bcrypt хеш")
}

func TestHashPassword_UniqueHashesForSamePassword(t *testing.T) {
	password := "SamePassword"

	hash1, err1 := HashPassword(password)
	hash2, err2 := HashPassword(password)

	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.NotEqual(t, hash1, hash2, "Два хеші одного пароля мають відрізнятися (різні salt)")
}

func TestHashPassword_EmptyPassword(t *testing.T) {
	// bcrypt допускає пустий пароль — це валідний вхід
	hashedPassword, err := HashPassword("")

	require.NoError(t, err)
	assert.NotEmpty(t, hashedPassword)
}

func TestHashPassword_LongPassword_ExceedingLimit(t *testing.T) {
	// bcrypt має ліміт 72 байти — паролі довші за це мають повертати помилку
	longPassword := make([]byte, 100)
	for i := range longPassword {
		longPassword[i] = 'A'
	}

	_, err := HashPassword(string(longPassword))

	assert.Error(t, err, "Пароль довший за 72 байти повинен повертати помилку")
	assert.Contains(t, err.Error(), "password length exceeds 72 bytes")
}

func TestHashPassword_ExactMaxLength(t *testing.T) {
	// Пароль рівно 72 байти — має працювати
	exactPassword := make([]byte, 72)
	for i := range exactPassword {
		exactPassword[i] = 'B'
	}

	hashedPassword, err := HashPassword(string(exactPassword))

	require.NoError(t, err)
	assert.NotEmpty(t, hashedPassword)
}

func TestHashPassword_UnicodePassword(t *testing.T) {
	password := "Пароль123!Ї"

	hashedPassword, err := HashPassword(password)

	require.NoError(t, err)
	assert.NotEmpty(t, hashedPassword)
}

func TestHashPassword_UsesDefaultCost(t *testing.T) {
	password := "TestCost"

	hashedPassword, err := HashPassword(password)
	require.NoError(t, err)

	cost, err := bcrypt.Cost([]byte(hashedPassword))
	require.NoError(t, err)
	assert.Equal(t, bcrypt.DefaultCost, cost, "Має використовувати bcrypt.DefaultCost")
}

// ==========================================
// CheckPassword Tests
// ==========================================

func TestCheckPassword_CorrectPassword(t *testing.T) {
	password := "CorrectPassword123!"

	hashedPassword, err := HashPassword(password)
	require.NoError(t, err)

	err = CheckPassword(password, hashedPassword)
	assert.NoError(t, err, "Правильний пароль має проходити перевірку")
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	password := "CorrectPassword123!"
	wrongPassword := "WrongPassword456!"

	hashedPassword, err := HashPassword(password)
	require.NoError(t, err)

	err = CheckPassword(wrongPassword, hashedPassword)
	assert.Error(t, err, "Неправильний пароль має повертати помилку")
	assert.ErrorIs(t, err, bcrypt.ErrMismatchedHashAndPassword)
}

func TestCheckPassword_EmptyPasswordAgainstHash(t *testing.T) {
	password := "SomePassword"

	hashedPassword, err := HashPassword(password)
	require.NoError(t, err)

	err = CheckPassword("", hashedPassword)
	assert.Error(t, err, "Пустий пароль не повинен збігатися з хешем непустого пароля")
}

func TestCheckPassword_EmptyPasswordMatchesEmptyHash(t *testing.T) {
	// Хешуємо пустий пароль і потім перевіряємо
	hashedPassword, err := HashPassword("")
	require.NoError(t, err)

	err = CheckPassword("", hashedPassword)
	assert.NoError(t, err, "Пустий пароль має збігатися зі своїм хешем")
}

func TestCheckPassword_InvalidHash(t *testing.T) {
	err := CheckPassword("anypassword", "not-a-valid-bcrypt-hash")
	assert.Error(t, err, "Невалідний хеш має повертати помилку")
}

func TestCheckPassword_CaseSensitive(t *testing.T) {
	password := "CaseSensitive"

	hashedPassword, err := HashPassword(password)
	require.NoError(t, err)

	err = CheckPassword("casesensitive", hashedPassword)
	assert.Error(t, err, "Пароль має бути чутливим до регістру")
}

func TestCheckPassword_SimilarPasswords(t *testing.T) {
	password := "MyPassword1"

	hashedPassword, err := HashPassword(password)
	require.NoError(t, err)

	// Дуже схожі паролі, але не ідентичні
	similarPasswords := []string{
		"MyPassword2",
		"MyPassword1 ",
		" MyPassword1",
		"myPassword1",
		"MyPassword",
	}

	for _, similar := range similarPasswords {
		err := CheckPassword(similar, hashedPassword)
		assert.Error(t, err, "Схожий, але відмінний пароль '%s' не повинен проходити перевірку", similar)
	}
}

// ==========================================
// Integration: HashPassword + CheckPassword
// ==========================================

func TestHashAndCheck_RoundTrip(t *testing.T) {
	passwords := []string{
		"simple",
		"C0mpl3x!P@$$w0rd",
		"Юнікод_пароль_🔐",
		"   spaces   ",
		"12345678",
		"aVeryLongPasswordThatExceedsNormalLengthExpectationsForMostSystems!!!",
	}

	for _, pw := range passwords {
		t.Run(pw, func(t *testing.T) {
			hash, err := HashPassword(pw)
			require.NoError(t, err)

			err = CheckPassword(pw, hash)
			assert.NoError(t, err, "Пароль '%s' має успішно проходити round-trip перевірку", pw)
		})
	}
}
