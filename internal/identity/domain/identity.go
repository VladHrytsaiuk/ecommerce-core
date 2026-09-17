package domain

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials         = errors.New("invalid credentials")
	ErrInvalidPassword            = errors.New("invalid password")
	ErrUserNotFound               = errors.New("identity user not found")
	ErrEmailAlreadyExists         = errors.New("identity email already exists")
	ErrPhoneAlreadyExists         = errors.New("identity phone already exists")
	ErrOAuthIdentityNotFound      = errors.New("OAuth identity not found")
	ErrOAuthIdentityAlreadyLinked = errors.New("OAuth identity is already linked")
	ErrOAuthProviderUnavailable   = errors.New("OAuth provider is unavailable")
	ErrInvalidOAuthState          = errors.New("invalid or expired OAuth state")
	ErrOAuthAccountLinkRequired   = errors.New("OAuth account linking is required")
	ErrInvalidProfile             = errors.New("invalid profile")
	ErrProfileConflict            = errors.New("profile update conflict")
	ErrProfileNotFound            = errors.New("profile not found")
	ErrAddressNotFound            = errors.New("customer address not found")
	ErrInvalidCustomerAddress     = errors.New("invalid customer address")
	ErrInvalidCustomerProfile     = errors.New("invalid customer profile")
)

// UserReader is the narrow Core-owned port required by login. It deliberately
// has no dependency on the retired legacy User repository.
type UserReader interface {
	FindByEmail(context.Context, string) (*User, error)
}

type LoginResult struct {
	AccessToken string
	ExpiresAt   time.Time
	Role        string
}

type Service interface {
	Login(context.Context, string, string) (*LoginResult, error)
}

// CustomerProfile is the stable, typed customer profile extension. Metadata is
// reserved for configured future fields and never contains credentials.
type CustomerProfile struct {
	UserID      uuid.UUID
	DateOfBirth *time.Time
	Gender      string
	Metadata    map[string]json.RawMessage
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CustomerAddress is owned by the customer profile module. Orders copy it to
// their own immutable delivery snapshot at checkout and never retain this ID.
type CustomerAddress struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Title     string
	Country   string
	City      string
	Line1     string
	Line2     string
	ZipCode   string
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CustomerProfileRepository interface {
	GetCustomerProfile(context.Context, uuid.UUID) (*CustomerProfile, error)
	UpsertCustomerProfile(context.Context, CustomerProfile) (*CustomerProfile, error)
	ListCustomerAddresses(context.Context, uuid.UUID) ([]CustomerAddress, error)
	CreateCustomerAddress(context.Context, CustomerAddress) (*CustomerAddress, error)
	UpdateCustomerAddress(context.Context, CustomerAddress) (*CustomerAddress, error)
	DeleteCustomerAddress(context.Context, uuid.UUID, uuid.UUID) error
}

// CustomerProfileService never accepts a user ID supplied by a transport
// payload: delivery handlers derive ownership solely from the JWT context.
type CustomerProfileService interface {
	GetCustomerProfile(context.Context, uuid.UUID) (*CustomerProfile, error)
	PatchCustomerProfile(context.Context, uuid.UUID, CustomerProfilePatch) (*CustomerProfile, error)
	ListCustomerAddresses(context.Context, uuid.UUID) ([]CustomerAddress, error)
	CreateCustomerAddress(context.Context, uuid.UUID, AddressInput) (*CustomerAddress, error)
	UpdateCustomerAddress(context.Context, uuid.UUID, uuid.UUID, AddressInput) (*CustomerAddress, error)
	DeleteCustomerAddress(context.Context, uuid.UUID, uuid.UUID) error
	GetAvailableProfileFields(context.Context, uuid.UUID) (map[string]bool, error)
}

type CustomerProfilePatch struct {
	DateOfBirth *time.Time
	Gender      *string
	Metadata    map[string]json.RawMessage
}

type AddressInput struct {
	Title     string
	Country   string
	City      string
	Line1     string
	Line2     string
	ZipCode   string
	IsDefault bool
}
