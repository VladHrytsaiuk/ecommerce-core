// Package postgres implements Admin RBAC read ports with PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) FindAuthorizationState(ctx context.Context, userID uuid.UUID) (domain.AuthorizationState, error) {
	if r == nil || r.db == nil || userID == uuid.Nil {
		return domain.AuthorizationState{}, domain.ErrNotAdmin
	}
	var record struct {
		UserID  uuid.UUID `gorm:"column:user_id"`
		Version int64     `gorm:"column:authorization_version"`
	}
	err := r.db.WithContext(ctx).
		Table("admin_users").
		Select("user_id, authorization_version").
		Where("user_id = ? AND is_active = TRUE", userID).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.AuthorizationState{}, domain.ErrNotAdmin
	}
	if err != nil {
		return domain.AuthorizationState{}, fmt.Errorf("find admin authorization state: %w", err)
	}
	return domain.AuthorizationState{UserID: record.UserID, Version: record.Version}, nil
}

func (r *Repository) ListPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	if r == nil || r.db == nil || userID == uuid.Nil {
		return nil, domain.ErrNotAdmin
	}
	var permissions []string
	err := r.db.WithContext(ctx).
		Table("permissions AS permission").
		Distinct("permission.code").
		Joins("JOIN role_permissions AS role_permission ON role_permission.permission_id = permission.id").
		Joins("JOIN admin_user_roles AS admin_role ON admin_role.role_id = role_permission.role_id").
		Joins("JOIN admin_users AS admin_user ON admin_user.user_id = admin_role.user_id AND admin_user.is_active = TRUE").
		Where("admin_role.user_id = ?", userID).
		Order("permission.code ASC").
		Pluck("permission.code", &permissions).Error
	if err != nil {
		return nil, fmt.Errorf("list admin permissions: %w", err)
	}
	return permissions, nil
}

var _ domain.AccessRepository = (*Repository)(nil)
