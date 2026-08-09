package service

import (
	"context"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/badges/domain"
	"github.com/google/uuid"
)

type Service struct{ repository domain.Repository }

func New(repository domain.Repository) *Service { return &Service{repository: repository} }
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*domain.Badge, error) {
	if s.repository == nil || id == uuid.Nil {
		return nil, domain.ErrInvalidBadge
	}
	return s.repository.Get(ctx, id)
}
func (s *Service) List(ctx context.Context) ([]domain.Badge, error) {
	if s.repository == nil {
		return nil, domain.ErrInvalidBadge
	}
	return s.repository.List(ctx)
}
func (s *Service) Create(ctx context.Context, command domain.CreateCommand) (*domain.Badge, error) {
	if !valid(&command.Slug, &command.Color, command.Translations) || s.repository == nil {
		return nil, domain.ErrInvalidBadge
	}
	return s.repository.Create(ctx, command)
}
func (s *Service) Update(ctx context.Context, id uuid.UUID, command domain.UpdateCommand) (*domain.Badge, error) {
	if id == uuid.Nil || s.repository == nil {
		return nil, domain.ErrInvalidBadge
	}
	create := domain.CreateCommand{Slug: command.Slug, Color: command.Color, Translations: command.Translations}
	if !valid(&create.Slug, &create.Color, create.Translations) {
		return nil, domain.ErrInvalidBadge
	}
	return s.repository.Update(ctx, id, command)
}
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if s.repository == nil || id == uuid.Nil {
		return domain.ErrInvalidBadge
	}
	return s.repository.Delete(ctx, id)
}
func (s *Service) AssignProduct(ctx context.Context, badgeID, productID uuid.UUID) error {
	if s.repository == nil || badgeID == uuid.Nil || productID == uuid.Nil {
		return domain.ErrInvalidBadge
	}
	return s.repository.AssignProduct(ctx, badgeID, productID)
}
func (s *Service) RemoveProduct(ctx context.Context, badgeID, productID uuid.UUID) error {
	if s.repository == nil || badgeID == uuid.Nil || productID == uuid.Nil {
		return domain.ErrInvalidBadge
	}
	return s.repository.RemoveProduct(ctx, badgeID, productID)
}
func valid(slug, color *string, translations []domain.Translation) bool {
	*slug = strings.ToLower(strings.TrimSpace(*slug))
	*color = strings.TrimSpace(*color)
	if *slug == "" || *color == "" || len(translations) == 0 {
		return false
	}
	seen := map[string]struct{}{}
	for i := range translations {
		translations[i].Locale = strings.ToLower(strings.TrimSpace(translations[i].Locale))
		translations[i].Name = strings.TrimSpace(translations[i].Name)
		if translations[i].Locale == "" || translations[i].Name == "" {
			return false
		}
		if _, ok := seen[translations[i].Locale]; ok {
			return false
		}
		seen[translations[i].Locale] = struct{}{}
	}
	return true
}

var _ domain.Service = (*Service)(nil)
