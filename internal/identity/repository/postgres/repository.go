// Package postgres implements identity persistence ports with GORM. Its DTOs
// are intentionally private so GORM and JSONB details do not leak into domain.
package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

type UserRepository struct{ db *gorm.DB }

func NewUserRepository(db *gorm.DB) *UserRepository { return &UserRepository{db: db} }

func (r *UserRepository) Create(ctx context.Context, user domain.NewUser) (*domain.User, error) {
	record := userRecord{ID: uuid.New(), Email: normalizeOptional(user.Email), Phone: normalizeOptional(user.Phone), PasswordHash: nullableString(user.PasswordHash), Role: string(user.Role), Status: string(user.Status)}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, mapUserError(err)
	}
	return record.toDomain(), nil
}

func (r *UserRepository) FindByLogin(ctx context.Context, login string) (*domain.User, error) {
	login = strings.TrimSpace(login)
	if login == "" {
		return nil, domain.ErrUserNotFound
	}
	var record userRecord
	if err := r.db.WithContext(ctx).Where("lower(email) = ? OR phone = ?", strings.ToLower(login), login).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return record.toDomain(), nil
}

func (r *UserRepository) FindByID(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	var record userRecord
	if err := r.db.WithContext(ctx).First(&record, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return record.toDomain(), nil
}

type OAuthIdentityRepository struct{ db *gorm.DB }

func NewOAuthIdentityRepository(db *gorm.DB) *OAuthIdentityRepository {
	return &OAuthIdentityRepository{db: db}
}

func (r *OAuthIdentityRepository) FindByProviderSubject(ctx context.Context, provider, subject string) (*domain.OAuthIdentity, error) {
	var record oauthIdentityRecord
	if err := r.db.WithContext(ctx).Where("provider = ? AND subject = ?", normalizeCode(provider), strings.TrimSpace(subject)).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrOAuthIdentityNotFound
		}
		return nil, err
	}
	return record.toDomain(), nil
}

func (r *OAuthIdentityRepository) Create(ctx context.Context, identity domain.OAuthIdentity) (*domain.OAuthIdentity, error) {
	record := oauthIdentityRecord{ID: identity.ID, UserID: identity.UserID, Provider: normalizeCode(identity.Provider), Subject: strings.TrimSpace(identity.Subject), ProviderEmail: normalizeOptional(identity.ProviderEmail), EmailVerified: identity.EmailVerified}
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, domain.ErrOAuthIdentityAlreadyLinked
		}
		return nil, err
	}
	return record.toDomain(), nil
}

type ProfileRepository struct{ db *gorm.DB }

func NewProfileRepository(db *gorm.DB) *ProfileRepository { return &ProfileRepository{db: db} }

func (r *ProfileRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (*domain.Profile, error) {
	var record profileRecord
	if err := r.db.WithContext(ctx).First(&record, "user_id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrProfileNotFound
		}
		return nil, err
	}
	return record.toDomain()
}

func (r *ProfileRepository) Upsert(ctx context.Context, profile domain.Profile) (*domain.Profile, error) {
	encoded, err := json.Marshal(profile.Attributes)
	if err != nil {
		return nil, fmt.Errorf("encode profile attributes: %w", err)
	}
	record := profileRecord{UserID: profile.UserID, Attributes: jsonb(encoded), SchemaVersion: profile.SchemaVersion}
	if profile.Revision == 0 {
		record.Revision = 1
		result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoNothing: true,
		}).Create(&record)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, domain.ErrProfileConflict
		}
	} else {
		result := r.db.WithContext(ctx).Model(&profileRecord{}).
			Where("user_id = ? AND revision = ?", profile.UserID, profile.Revision).
			Updates(map[string]any{
				"attributes":     record.Attributes,
				"schema_version": record.SchemaVersion,
				"revision":       gorm.Expr("revision + 1"),
				"updated_at":     time.Now().UTC(),
			})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, domain.ErrProfileConflict
		}
	}
	return r.FindByUserID(ctx, profile.UserID)
}

type OAuthAttemptStore struct{ db *gorm.DB }

func NewOAuthAttemptStore(db *gorm.DB) *OAuthAttemptStore { return &OAuthAttemptStore{db: db} }

func (r *OAuthAttemptStore) Create(ctx context.Context, attempt domain.OAuthAttempt) error {
	record := oauthAttemptRecord{ID: uuid.New(), Provider: normalizeCode(attempt.Provider), StateHash: stateDigest(attempt.State), RedirectURI: strings.TrimSpace(attempt.RedirectURI), Nonce: attempt.Nonce, CodeVerifier: attempt.CodeVerifier, ExpiresAt: attempt.ExpiresAt}
	return r.db.WithContext(ctx).Create(&record).Error
}

func (r *OAuthAttemptStore) Consume(ctx context.Context, provider, state string, now time.Time) (*domain.OAuthAttempt, error) {
	var record oauthAttemptRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&oauthAttemptRecord{}).Where("provider = ? AND state_hash = ? AND consumed_at IS NULL AND expires_at > ?", normalizeCode(provider), stateDigest(state), now)
		result := query.Update("consumed_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrInvalidOAuthState
		}
		if err := tx.Where("provider = ? AND state_hash = ?", normalizeCode(provider), stateDigest(state)).First(&record).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return record.toDomain(), nil
}

// AuthTransaction wires independent repository ports to the same local
// PostgreSQL transaction. It never executes provider HTTP calls.
type AuthTransaction struct{ db *gorm.DB }

func NewAuthTransaction(db *gorm.DB) *AuthTransaction { return &AuthTransaction{db: db} }

func (r *AuthTransaction) WithinTransaction(ctx context.Context, fn func(domain.UserRepository, domain.OAuthIdentityRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(NewUserRepository(tx), NewOAuthIdentityRepository(tx))
	})
}

type userRecord struct {
	ID           uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	Email        *string   `gorm:"column:email"`
	Phone        *string   `gorm:"column:phone"`
	PasswordHash *string   `gorm:"column:password_hash"`
	Role         string    `gorm:"column:role"`
	Status       string    `gorm:"column:status"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (userRecord) TableName() string { return "users" }

func (record userRecord) toDomain() *domain.User {
	passwordHash := ""
	if record.PasswordHash != nil {
		passwordHash = *record.PasswordHash
	}
	return &domain.User{ID: record.ID, Email: record.Email, Phone: record.Phone, PasswordHash: passwordHash, Role: domain.Role(record.Role), Status: domain.UserStatus(record.Status), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

type oauthIdentityRecord struct {
	ID            uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	UserID        uuid.UUID `gorm:"column:user_id;type:uuid"`
	Provider      string    `gorm:"column:provider"`
	Subject       string    `gorm:"column:subject"`
	ProviderEmail *string   `gorm:"column:provider_email"`
	EmailVerified bool      `gorm:"column:email_verified"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (oauthIdentityRecord) TableName() string { return "user_oauth_identities" }

func (record oauthIdentityRecord) toDomain() *domain.OAuthIdentity {
	return &domain.OAuthIdentity{ID: record.ID, UserID: record.UserID, Provider: record.Provider, Subject: record.Subject, ProviderEmail: record.ProviderEmail, EmailVerified: record.EmailVerified, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

type profileRecord struct {
	UserID        uuid.UUID `gorm:"column:user_id;type:uuid;primaryKey"`
	Attributes    jsonb     `gorm:"column:attributes;type:jsonb"`
	SchemaVersion int       `gorm:"column:schema_version"`
	Revision      int       `gorm:"column:revision"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (profileRecord) TableName() string { return "user_profiles" }

func (record profileRecord) toDomain() (*domain.Profile, error) {
	attributes := make(map[string]json.RawMessage)
	if err := json.Unmarshal(record.Attributes, &attributes); err != nil {
		return nil, fmt.Errorf("decode profile attributes: %w", err)
	}
	return &domain.Profile{UserID: record.UserID, Attributes: attributes, SchemaVersion: record.SchemaVersion, Revision: record.Revision, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}

type oauthAttemptRecord struct {
	ID           uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	Provider     string     `gorm:"column:provider"`
	StateHash    []byte     `gorm:"column:state_hash"`
	RedirectURI  string     `gorm:"column:redirect_uri"`
	Nonce        string     `gorm:"column:nonce"`
	CodeVerifier string     `gorm:"column:code_verifier"`
	ExpiresAt    time.Time  `gorm:"column:expires_at"`
	ConsumedAt   *time.Time `gorm:"column:consumed_at"`
}

func (oauthAttemptRecord) TableName() string { return "oauth_authorization_attempts" }

func (record oauthAttemptRecord) toDomain() *domain.OAuthAttempt {
	return &domain.OAuthAttempt{Provider: record.Provider, RedirectURI: record.RedirectURI, Nonce: record.Nonce, CodeVerifier: record.CodeVerifier, ExpiresAt: record.ExpiresAt}
}

// jsonb implements driver.Valuer/sql.Scanner so GORM passes JSON to pgx as a
// JSONB value rather than serializing a Go byte slice as base64 text.
type jsonb []byte

func (value jsonb) Value() (driver.Value, error) {
	if !json.Valid(value) {
		return nil, fmt.Errorf("invalid JSONB")
	}
	return []byte(value), nil
}

func (value *jsonb) Scan(source any) error {
	switch data := source.(type) {
	case []byte:
		*value = append((*value)[:0], data...)
	case string:
		*value = append((*value)[:0], data...)
	case nil:
		*value = nil
	default:
		return fmt.Errorf("cannot scan %T into JSONB", source)
	}
	if !json.Valid(*value) {
		return fmt.Errorf("invalid JSONB from database")
	}
	return nil
}

func normalizeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	if strings.Contains(normalized, "@") {
		normalized = strings.ToLower(normalized)
	}
	return &normalized
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func normalizeCode(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func stateDigest(state string) []byte {
	digest := sha256.Sum256([]byte(state))
	return digest[:]
}

func mapUserError(err error) error {
	if !isUniqueViolation(err) {
		return err
	}
	message := err.Error()
	if strings.Contains(message, "users_phone") {
		return domain.ErrPhoneAlreadyExists
	}
	return domain.ErrEmailAlreadyExists
}

func isUniqueViolation(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}

var _ domain.UserRepository = (*UserRepository)(nil)
var _ domain.OAuthIdentityRepository = (*OAuthIdentityRepository)(nil)
var _ domain.ProfileRepository = (*ProfileRepository)(nil)
var _ domain.OAuthAttemptStore = (*OAuthAttemptStore)(nil)
var _ domain.AuthTransaction = (*AuthTransaction)(nil)
