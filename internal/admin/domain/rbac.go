// Package domain contains the provider-neutral Admin RBAC contracts.
package domain

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrNotAdmin          = errors.New("admin access is not active")
	ErrPermissionDenied  = errors.New("admin permission denied")
	ErrInvalidPermission = errors.New("invalid permission")
)

// AuthorizationState is deliberately small enough to read on every request.
// Its version fences stale cache entries after a role/permission mutation.
type AuthorizationState struct {
	UserID  uuid.UUID
	Version int64
}

type AccessRepository interface {
	FindAuthorizationState(context.Context, uuid.UUID) (AuthorizationState, error)
	ListPermissions(context.Context, uuid.UUID) ([]string, error)
}

type Authorizer interface {
	Require(context.Context, uuid.UUID, string) error
}
