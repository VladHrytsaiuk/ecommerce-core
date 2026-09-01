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
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

type UserRepository struct{ db *gorm.DB }

func NewUserRepository(db *gorm.DB) *UserRepository { return &UserRepository{db: db} }

// VerificationStatusReader performs a deliberately narrow projection. In
// particular it must not hydrate password_hash merely to answer Checkout's
// verification policy.
type VerificationStatusReader struct{ db *gorm.DB }

func NewVerificationStatusReader(db *gorm.DB) *VerificationStatusReader {
	return &VerificationStatusReader{db: db}
}

func (r *VerificationStatusReader) GetVerificationStatus(ctx context.Context, userID uuid.UUID) (domain.UserVerificationStatus, error) {
	var record struct {
		EmailVerified bool `gorm:"column:email_verified"`
		PhoneVerified bool `gorm:"column:phone_verified"`
	}
	if err := r.db.WithContext(ctx).Table("users").Select("email_verified", "phone_verified").Where("id = ?", userID).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.UserVerificationStatus{}, domain.ErrUserNotFound
		}
		return domain.UserVerificationStatus{}, err
	}
	return domain.UserVerificationStatus{EmailVerified: record.EmailVerified, PhoneVerified: record.PhoneVerified}, nil
}

func (r *UserRepository) Create(ctx context.Context, user domain.NewUser) (*domain.User, error) {
	record := userRecord{ID: uuid.New(), Email: normalizeOptional(user.Email), Phone: normalizeOptional(user.Phone), EmailVerified: user.EmailVerified, PhoneVerified: user.PhoneVerified, PasswordHash: nullableString(user.PasswordHash), Role: string(user.Role), Status: string(user.Status)}
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

// CustomerProfileRepository owns the typed address-book/profile extension.
// It is intentionally distinct from the legacy generic ProfileRepository.
type CustomerProfileRepository struct{ db *gorm.DB }

func NewCustomerProfileRepository(db *gorm.DB) *CustomerProfileRepository {
	return &CustomerProfileRepository{db: db}
}

func (r *CustomerProfileRepository) GetCustomerProfile(ctx context.Context, userID uuid.UUID) (*domain.CustomerProfile, error) {
	var record customerProfileRecord
	if err := r.database(ctx).First(&record, "user_id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrProfileNotFound
		}
		return nil, err
	}
	return record.toDomain()
}

func (r *CustomerProfileRepository) UpsertCustomerProfile(ctx context.Context, profile domain.CustomerProfile) (*domain.CustomerProfile, error) {
	encoded, err := json.Marshal(profile.Metadata)
	if err != nil {
		return nil, fmt.Errorf("encode customer profile metadata: %w", err)
	}
	record := customerProfileRecord{UserID: profile.UserID, DateOfBirth: profile.DateOfBirth, Gender: nullableString(profile.Gender), Metadata: jsonb(encoded)}
	db := r.database(ctx)
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{"date_of_birth": record.DateOfBirth, "gender": record.Gender, "metadata": record.Metadata, "updated_at": time.Now().UTC()}),
	}).Create(&record).Error; err != nil {
		return nil, err
	}
	return r.GetCustomerProfile(ctx, profile.UserID)
}

func (r *CustomerProfileRepository) ListCustomerAddresses(ctx context.Context, userID uuid.UUID) ([]domain.CustomerAddress, error) {
	var records []customerAddressRecord
	if err := r.database(ctx).Where("user_id = ?", userID).Order("is_default DESC, created_at DESC, id DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	addresses := make([]domain.CustomerAddress, 0, len(records))
	for _, record := range records {
		addresses = append(addresses, record.toDomain())
	}
	return addresses, nil
}

func (r *CustomerProfileRepository) CreateCustomerAddress(ctx context.Context, address domain.CustomerAddress) (*domain.CustomerAddress, error) {
	record := customerAddressRecordFromDomain(address)
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	err := transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		if record.IsDefault {
			if err := tx.Model(&customerAddressRecord{}).Where("user_id = ? AND is_default", record.UserID).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&record).Error
	})
	if err != nil {
		return nil, err
	}
	result := record.toDomain()
	return &result, nil
}

func (r *CustomerProfileRepository) UpdateCustomerAddress(ctx context.Context, address domain.CustomerAddress) (*domain.CustomerAddress, error) {
	record := customerAddressRecordFromDomain(address)
	err := transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		if record.IsDefault {
			if err := tx.Model(&customerAddressRecord{}).Where("user_id = ? AND id <> ? AND is_default", record.UserID, record.ID).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		result := tx.Model(&customerAddressRecord{}).Where("id = ? AND user_id = ?", record.ID, record.UserID).Updates(map[string]any{
			"title": record.Title, "country": record.Country, "city": record.City, "line1": record.Line1, "line2": record.Line2, "zip_code": record.ZipCode, "is_default": record.IsDefault, "updated_at": time.Now().UTC(),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrAddressNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &address, nil
}

func (r *CustomerProfileRepository) DeleteCustomerAddress(ctx context.Context, userID, addressID uuid.UUID) error {
	result := r.database(ctx).Where("id = ? AND user_id = ?", addressID, userID).Delete(&customerAddressRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrAddressNotFound
	}
	return nil
}

func (r *CustomerProfileRepository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

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
	err := transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
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
	return transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		return fn(NewUserRepository(tx), NewOAuthIdentityRepository(tx))
	})
}

type userRecord struct {
	ID            uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	Email         *string   `gorm:"column:email"`
	Phone         *string   `gorm:"column:phone"`
	EmailVerified bool      `gorm:"column:email_verified"`
	PhoneVerified bool      `gorm:"column:phone_verified"`
	PasswordHash  *string   `gorm:"column:password_hash"`
	Role          string    `gorm:"column:role"`
	Status        string    `gorm:"column:status"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (userRecord) TableName() string { return "users" }

func (record userRecord) toDomain() *domain.User {
	passwordHash := ""
	if record.PasswordHash != nil {
		passwordHash = *record.PasswordHash
	}
	return &domain.User{ID: record.ID, Email: record.Email, Phone: record.Phone, EmailVerified: record.EmailVerified, PhoneVerified: record.PhoneVerified, PasswordHash: passwordHash, Role: domain.Role(record.Role), Status: domain.UserStatus(record.Status), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
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

type customerProfileRecord struct {
	UserID      uuid.UUID  `gorm:"column:user_id;type:uuid;primaryKey"`
	DateOfBirth *time.Time `gorm:"column:date_of_birth"`
	Gender      *string    `gorm:"column:gender"`
	Metadata    jsonb      `gorm:"column:metadata;type:jsonb"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

func (customerProfileRecord) TableName() string { return "customer_profiles" }

func (record customerProfileRecord) toDomain() (*domain.CustomerProfile, error) {
	metadata := make(map[string]json.RawMessage)
	if err := json.Unmarshal(record.Metadata, &metadata); err != nil {
		return nil, fmt.Errorf("decode customer profile metadata: %w", err)
	}
	gender := ""
	if record.Gender != nil {
		gender = *record.Gender
	}
	return &domain.CustomerProfile{UserID: record.UserID, DateOfBirth: record.DateOfBirth, Gender: gender, Metadata: metadata, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}

type customerAddressRecord struct {
	ID        uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	UserID    uuid.UUID `gorm:"column:user_id;type:uuid"`
	Title     string    `gorm:"column:title"`
	Country   string    `gorm:"column:country"`
	City      string    `gorm:"column:city"`
	Line1     string    `gorm:"column:line1"`
	Line2     *string   `gorm:"column:line2"`
	ZipCode   string    `gorm:"column:zip_code"`
	IsDefault bool      `gorm:"column:is_default"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (customerAddressRecord) TableName() string { return "customer_addresses" }

func (record customerAddressRecord) toDomain() domain.CustomerAddress {
	line2 := ""
	if record.Line2 != nil {
		line2 = *record.Line2
	}
	return domain.CustomerAddress{ID: record.ID, UserID: record.UserID, Title: record.Title, Country: record.Country, City: record.City, Line1: record.Line1, Line2: line2, ZipCode: record.ZipCode, IsDefault: record.IsDefault, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

func customerAddressRecordFromDomain(address domain.CustomerAddress) customerAddressRecord {
	return customerAddressRecord{ID: address.ID, UserID: address.UserID, Title: address.Title, Country: address.Country, City: address.City, Line1: address.Line1, Line2: nullableString(address.Line2), ZipCode: address.ZipCode, IsDefault: address.IsDefault, CreatedAt: address.CreatedAt, UpdatedAt: address.UpdatedAt}
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
