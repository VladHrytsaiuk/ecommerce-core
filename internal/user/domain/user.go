package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ==========================================
// Domain Errors (Доменні помилки)
// ==========================================

const (
	RoleCustomer = 1
	RoleAdmin    = 2
	RoleOwner    = 3
)

func IsAdminRole(roleID int) bool {
	return roleID == RoleAdmin || roleID == RoleOwner
}

var (
	ErrUserNotFound         = errors.New("user not found")
	ErrEmailAlreadyExists   = errors.New("email already exists")
	ErrPhoneAlreadyExists   = errors.New("phone already exists")
	ErrSessionNotFound      = errors.New("session not found")
	ErrInvalidCredentials   = errors.New("invalid email or password")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrSessionBlocked       = errors.New("session is blocked")
	ErrSessionExpired       = errors.New("session is expired")
	ErrInternal             = errors.New("internal server error")
	ErrInvalidCode          = errors.New("invalid verification code")
	ErrCodeExpired          = errors.New("verification code expired")
	ErrUserNotVerified      = errors.New("email is not verified")
	ErrNewPasswordSameAsOld = errors.New("new password cannot be the same as the old one")
	ErrInvalidGoogleToken   = errors.New("invalid google id token")
	ErrForbiddenRole        = errors.New("access denied: insufficient permissions")
	ErrPhoneNotVerified     = errors.New("phone number is not verified")
	ErrTooManyAttempts      = errors.New("too many verify attempts")
)

// ==========================================
// Entities (Сутності)
// ==========================================

// User мапиться на таблицю "user" в БД.
type User struct {
	ID              uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	RoleID          int            `gorm:"not null" json:"role_id"`
	FirstName       string         `gorm:"type:varchar(100);not null" json:"first_name"`
	LastName        string         `gorm:"type:varchar(100);not null" json:"last_name"`
	PasswordHash    string         `gorm:"type:varchar(255);not null" json:"-"`
	Email           string         `gorm:"type:varchar(255);not null;unique" json:"email"`
	Phone           *string        `gorm:"type:varchar(20);unique;default:null" json:"phone,omitempty"`
	IsEmailVerified bool           `gorm:"not null;default:false" json:"is_email_verified"`
	IsPhoneVerified bool           `gorm:"not null;default:false" json:"is_phone_verified"`
	WantsNewsletter bool           `gorm:"not null;default:false" json:"wants_newsletter"`
	IsBlocked       bool           `gorm:"not null;default:false" json:"is_blocked"`
	AuthProvider    string         `gorm:"type:varchar(50);not null;default:'local'" json:"auth_provider"`
	AvatarURL       string         `gorm:"type:varchar(255)" json:"avatar_url"`
	LastLoginAt     *time.Time     `json:"last_login_at"`
	CreatedAt       time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// UpdateMeInput містить поля для часткового оновлення профілю (PATCH).
type UpdateMeInput struct {
	FirstName       *string
	LastName        *string
	Phone           *string
	WantsNewsletter *bool
}

// TableName явно вказує назву таблиці для GORM (захист від резервованого слова "user")
func (User) TableName() string {
	return `"user"`
}

// Session мапиться на таблицю "session"
type Session struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID       uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	RefreshToken string    `gorm:"type:text;not null;unique" json:"refresh_token"`
	UserAgent    *string   `gorm:"type:text" json:"user_agent"`
	ClientIP     *string   `gorm:"type:varchar(45)" json:"client_ip"`
	IsBlocked    bool      `gorm:"not null;default:false" json:"is_blocked"`
	ExpiresAt    time.Time `gorm:"not null" json:"expires_at"`
	CreatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	// Relational data
	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (Session) TableName() string {
	return "session"
}

// UserAddress мапиться на таблицю "user_address"
type UserAddress struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID        uuid.UUID      `gorm:"type:uuid;not null" json:"user_id"`
	AddressName   string         `gorm:"type:varchar(255)" json:"address_name"`
	Provider      string         `gorm:"type:varchar(50);not null" json:"provider"`
	DeliveryType  string         `gorm:"type:varchar(50);not null" json:"delivery_type"`
	FullAddress   string         `gorm:"type:varchar(255);not null" json:"full_address"`
	CityRef       string         `gorm:"type:varchar(100);not null" json:"city_ref"`
	CityName      string         `gorm:"type:varchar(100);not null" json:"city_name"`
	AreaRef       string         `gorm:"type:varchar(100)" json:"area_ref"`
	AreaName      string         `gorm:"type:varchar(100)" json:"area_name"`
	WarehouseRef  string         `gorm:"type:varchar(100);not null" json:"warehouse_ref"`
	WarehouseName string         `gorm:"type:varchar(255);not null" json:"warehouse_name"`
	IsDefault     bool           `gorm:"not null;default:false" json:"is_default"`
	CreatedAt     time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

func (UserAddress) TableName() string {
	return "user_address"
}

// VerifyCode мапиться на таблицю "verify_code"
type VerifyCode struct {
	ID        uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    *uuid.UUID `gorm:"type:uuid;default:null" json:"user_id"`
	Target    string     `gorm:"type:varchar(255);not null" json:"target"`
	Type      string     `gorm:"type:varchar(50);not null" json:"type"` // e.g., "email_verification", "password_reset", "phone_verification"
	Code      string     `gorm:"type:varchar(10);not null" json:"code"`
	Attempts  int        `gorm:"not null;default:0" json:"attempts"`
	IsUsed    bool       `gorm:"not null;default:false" json:"is_used"`
	ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
	CreatedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (VerifyCode) TableName() string {
	return "verify_code"
}

// ==========================================
// Contracts (Інтерфейси Репозиторіїв)
// ==========================================

// UserRepository визначає контракт для збереження та пошуку користувачів.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	FindAllByRole(ctx context.Context, roleID int) ([]User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByPhone(ctx context.Context, phone string) (*User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id uuid.UUID) error
	Atomic(ctx context.Context, fn func(UserRepository, VerifyCodeRepository) error) error
}

// SessionRepository визначає контракт для операцій з сесіями (Refresh токенами).
type SessionRepository interface {
	Create(ctx context.Context, session *Session) error
	FindByToken(ctx context.Context, token string) (*Session, error)
	DeleteByToken(ctx context.Context, token string) error
	DeleteAllForUser(ctx context.Context, userID uuid.UUID) error
	DeleteExpired(ctx context.Context) error
	DeleteOldest(ctx context.Context, userID uuid.UUID, keepCount int) error
}

// UserAddressRepository визначає контракт для збереження адрес користувача.
type UserAddressRepository interface {
	Create(ctx context.Context, address *UserAddress) error
	FindAllByUserID(ctx context.Context, userID uuid.UUID) ([]UserAddress, error)
	FindByID(ctx context.Context, id uuid.UUID) (*UserAddress, error)
	Update(ctx context.Context, address *UserAddress) error
	Delete(ctx context.Context, id uuid.UUID) error
	UnsetDefaultAllForUser(ctx context.Context, userID uuid.UUID) error
	Atomic(ctx context.Context, fn func(UserAddressRepository) error) error
}

// VerifyCodeRepository визначає контракт для збереження та перевірки кодів.
type VerifyCodeRepository interface {
	Create(ctx context.Context, code *VerifyCode) error
	FindLastByTarget(ctx context.Context, target string, codeType string) (*VerifyCode, error)
	FindByCode(ctx context.Context, code string, codeType string) (*VerifyCode, error)
	MarkAsUsed(ctx context.Context, id uuid.UUID) error
	UpdateAttempts(ctx context.Context, id uuid.UUID, attempts int) error
	DeleteExpired(ctx context.Context) error
}

// ==========================================
// Service Interfaces (Інтерфейси Сервісів)
// ==========================================

// AuthService визначає бізнес-логіку автентифікації.
type AuthService interface {
	Register(ctx context.Context, user *User) (*AuthResponseData, error)
	CreateAdmin(ctx context.Context, user *User) (*User, error)
	ListAdmins(ctx context.Context) ([]User, error)
	DeleteAdmin(ctx context.Context, actorID, userID uuid.UUID) error
	Login(ctx context.Context, email, password, userAgent, clientIP string, requiredRole int) (*AuthResponseData, error)
	LoginWithGoogle(ctx context.Context, idToken, userAgent, clientIP string) (*AuthResponseData, error)
	Refresh(ctx context.Context, refreshToken, userAgent, clientIP string, requiredRole int) (*AuthResponseData, error)
	Logout(ctx context.Context, refreshToken string) error

	// Верифікація
	VerifyEmail(ctx context.Context, email, code string, userAgent, clientIP string) (*AuthResponseData, error)
	ResendVerificationCode(ctx context.Context, email string) error

	// Відновлення пароля
	ForgotPassword(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, email, code, newPassword string) error
	SetupPassword(ctx context.Context, setupToken, newPassword, userAgent, clientIP string) (*AuthResponseData, error)

	// Верифікація телефону
	RequestPhoneVerification(ctx context.Context, phone string, userID *uuid.UUID) (string, error) // повертає згенерований код (для розробки)
	ConfirmPhoneVerification(ctx context.Context, phone, code string) error
}

// WSNotifyService визначає контракт для відправки сповіщень у реальному часі.
type WSNotifyService interface {
	NotifyUser(userID uuid.UUID, data interface{}) error
}

// UserService визначає бізнес-логіку роботи з профілем та адресами.
type UserService interface {
	GetMe(ctx context.Context, userID uuid.UUID) (*User, error)
	UpdateMe(ctx context.Context, userID uuid.UUID, input *UpdateMeInput) (*User, error)
	DeleteMe(ctx context.Context, userID uuid.UUID) error

	// Адреси
	GetAddresses(ctx context.Context, userID uuid.UUID) ([]UserAddress, error)
	CreateAddress(ctx context.Context, address *UserAddress) (*UserAddress, error)
	UpdateAddress(ctx context.Context, userID uuid.UUID, addressID uuid.UUID, address *UserAddress) (*UserAddress, error)
	DeleteAddress(ctx context.Context, userID uuid.UUID, addressID uuid.UUID) error
}

// AuthResponseData структура для повернення токенів та даних юзера після входу/реєстрації.
type AuthResponseData struct {
	AccessToken  string
	RefreshToken string
	User         User
}
