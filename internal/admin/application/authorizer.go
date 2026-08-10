// Package application implements Admin authorization policies over narrow ports.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/cache"
)

const defaultPermissionsTTL = 5 * time.Minute

type Authorizer struct {
	repository domain.AccessRepository
	cache      cache.Service
	ttl        time.Duration
}

func NewAuthorizer(repository domain.AccessRepository, service cache.Service, ttl time.Duration) (*Authorizer, error) {
	if repository == nil {
		return nil, fmt.Errorf("RBAC access repository is required")
	}
	if service == nil {
		service = cache.NewNoOpService()
	}
	if ttl <= 0 {
		ttl = defaultPermissionsTTL
	}
	return &Authorizer{repository: repository, cache: service, ttl: ttl}, nil
}

func (a *Authorizer) Require(ctx context.Context, userID uuid.UUID, permission string) error {
	permission = strings.TrimSpace(permission)
	if userID == uuid.Nil {
		return domain.ErrNotAdmin
	}
	if permission == "" {
		return domain.ErrInvalidPermission
	}

	state, err := a.repository.FindAuthorizationState(ctx, userID)
	if err != nil {
		return err
	}
	if state.UserID != userID || state.Version <= 0 {
		return domain.ErrNotAdmin
	}

	permissions, err := a.permissions(ctx, state)
	if err != nil {
		return err
	}
	if _, allowed := permissions[permission]; !allowed {
		return domain.ErrPermissionDenied
	}
	return nil
}

func (a *Authorizer) permissions(ctx context.Context, state domain.AuthorizationState) (map[string]struct{}, error) {
	key := cacheKey(state)
	if raw, err := a.cache.Get(ctx, key); err == nil {
		var values []string
		if json.Unmarshal(raw, &values) == nil {
			return toSet(values), nil
		}
		_ = a.cache.Delete(ctx, key)
	} else if !errors.Is(err, cache.ErrMiss) {
		// Cache availability must never decide whether an authenticated admin has
		// a permission; PostgreSQL remains authoritative.
	}

	values, err := a.repository.ListPermissions(ctx, state.UserID)
	if err != nil {
		return nil, err
	}
	values = normalizePermissions(values)
	if raw, marshalErr := json.Marshal(values); marshalErr == nil {
		_ = a.cache.Set(ctx, key, raw, a.ttl)
	}
	return toSet(values), nil
}

func cacheKey(state domain.AuthorizationState) string {
	return fmt.Sprintf("rbac:v1:admin:%s:version:%d", state.UserID, state.Version)
}

func normalizePermissions(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func toSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

var _ domain.Authorizer = (*Authorizer)(nil)
