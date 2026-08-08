// Package domain defines provider-neutral identity and optional profile
// contracts. It does not import HTTP, GORM, provider SDKs, or adapters.
package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleCustomer Role = "customer"
	RoleManager  Role = "manager"
	RoleAdmin    Role = "admin"
	RoleOwner    Role = "owner"
)

type UserStatus string

const (
	UserStatusActive              UserStatus = "active"
	UserStatusDisabled            UserStatus = "disabled"
	UserStatusPendingVerification UserStatus = "pending_verification"
)

// User is the strict Core identity aggregate. Optional, store-specific data
// belongs to Profile, never to this type or the users table.
//
// Persistence adapters own their DTOs, therefore this domain entity has no
// GORM tags. PasswordHash must never be mapped directly to an HTTP response.
type User struct {
	ID           uuid.UUID
	Email        *string
	Phone        *string
	PasswordHash string
	Role         Role
	Status       UserStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// OAuthIdentity is the immutable provider subject linked to a Core user.
// Provider tokens are credentials and must not be persisted in this entity.
type OAuthIdentity struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Provider      string
	Subject       string
	ProviderEmail *string
	EmailVerified bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Profile is an optional, module-owned extension of User. json.RawMessage
// preserves JSON scalar types while ProfilePolicy validates the allowed keys.
type Profile struct {
	UserID        uuid.UUID
	Attributes    map[string]json.RawMessage
	SchemaVersion int
	Revision      int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ProfileFieldType string

const (
	ProfileFieldString ProfileFieldType = "string"
	ProfileFieldNumber ProfileFieldType = "number"
	ProfileFieldDate   ProfileFieldType = "date"
	ProfileFieldBool   ProfileFieldType = "bool"
	ProfileFieldEnum   ProfileFieldType = "enum"
)

// ProfileFieldConfig is deployment policy, parsed into typed StoreConfig and
// validated at Bootstrap. It is not a client-controlled schema.
type ProfileFieldConfig struct {
	Key              string           `json:"key"`
	Type             ProfileFieldType `json:"type"`
	Required         bool             `json:"required"`
	CustomerWritable bool             `json:"customer_writable"`
	Searchable       bool             `json:"searchable"`
	MaxLength        int              `json:"max_length"`
	AllowedValues    []string         `json:"allowed_values"`
}

type ProfilePolicy struct {
	SchemaVersion int                  `json:"schema_version"`
	Fields        []ProfileFieldConfig `json:"fields"`
}
