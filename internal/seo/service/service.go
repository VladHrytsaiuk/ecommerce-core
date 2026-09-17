package service

import (
	"context"
	"strings"
	"unicode/utf8"

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

// Column widths from migrations/modules/seo/000001. keywords and og_image_ref
// are TEXT and therefore unbounded in the schema, but an unbounded body is
// still worth refusing: a megabyte of keywords is not metadata.
const (
	maxResourceTypeRunes = 64
	maxSEOLocaleRunes    = 10
	maxTitleRunes        = 255
	maxDescriptionRunes  = 500
	maxKeywordsRunes     = 2000
	maxImageRefRunes     = 2000
)

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
	// Checked here so a value that is only too long is refused as invalid
	// input rather than reaching the column and failing the transaction with a
	// message that names neither the field nor the limit.
	if utf8.RuneCountInString(command.ResourceType) > maxResourceTypeRunes ||
		utf8.RuneCountInString(command.Locale) > maxSEOLocaleRunes ||
		utf8.RuneCountInString(command.Title) > maxTitleRunes ||
		utf8.RuneCountInString(command.Description) > maxDescriptionRunes ||
		utf8.RuneCountInString(command.Keywords) > maxKeywordsRunes ||
		utf8.RuneCountInString(command.OGImageRef) > maxImageRefRunes {
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
