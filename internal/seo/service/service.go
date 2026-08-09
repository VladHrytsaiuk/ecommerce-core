package service

import (
	"context"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/seo/domain"
	"github.com/google/uuid"
)

type Service struct{ repository domain.Repository }

func New(repository domain.Repository) *Service { return &Service{repository: repository} }

func (s *Service) Get(ctx context.Context, resourceType string, resourceID uuid.UUID, locale string) (*domain.Metadata, error) {
	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	locale = strings.ToLower(strings.TrimSpace(locale))
	if s.repository == nil || resourceType == "" || resourceID == uuid.Nil || locale == "" {
		return nil, domain.ErrInvalidMetadata
	}
	return s.repository.Get(ctx, resourceType, resourceID, locale)
}

func (s *Service) Upsert(ctx context.Context, command domain.UpsertCommand) (*domain.Metadata, error) {
	command.ResourceType = strings.ToLower(strings.TrimSpace(command.ResourceType))
	command.Locale = strings.ToLower(strings.TrimSpace(command.Locale))
	command.Title = strings.TrimSpace(command.Title)
	command.Description = strings.TrimSpace(command.Description)
	command.Keywords = strings.TrimSpace(command.Keywords)
	command.OGImageRef = strings.TrimSpace(command.OGImageRef)
	if s.repository == nil || command.ResourceType == "" || command.ResourceID == uuid.Nil || command.Locale == "" {
		return nil, domain.ErrInvalidMetadata
	}
	return s.repository.Upsert(ctx, command)
}
func (s *Service) Delete(ctx context.Context, resourceType string, resourceID uuid.UUID, locale string) error {
	if s.repository == nil || strings.TrimSpace(resourceType) == "" || resourceID == uuid.Nil || strings.TrimSpace(locale) == "" {
		return domain.ErrInvalidMetadata
	}
	return s.repository.Delete(ctx, strings.ToLower(strings.TrimSpace(resourceType)), resourceID, strings.ToLower(strings.TrimSpace(locale)))
}

var _ domain.Service = (*Service)(nil)
