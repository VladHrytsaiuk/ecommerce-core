package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	Role         string
	Status       string
}

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
