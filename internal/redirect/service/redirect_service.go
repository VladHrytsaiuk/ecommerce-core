package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/domain"
)

type redirectService struct {
	repo domain.RedirectRepository
	l    logger.Logger
}

func NewRedirectService(repo domain.RedirectRepository, l logger.Logger) domain.RedirectService {
	return &redirectService{
		repo: repo,
		l:    l,
	}
}

func (s *redirectService) RecordSlugChange(ctx context.Context, entityType string, entityID uuid.UUID, oldSlug string) error {
	// If the old slug is empty, there is nothing to redirect from
	if oldSlug == "" {
		return nil
	}

	history := &domain.UrlHistory{
		EntityType: entityType,
		EntityID:   entityID,
		OldSlug:    oldSlug,
	}

	if err := s.repo.SaveHistory(ctx, history); err != nil {
		s.l.Errorw("Failed to save slug history", "error", err, "entity_type", entityType, "entity_id", entityID, "old_slug", oldSlug)
		return err
	}

	return nil
}

func (s *redirectService) ResolveRedirect(ctx context.Context, oldSlug string, lang string) (string, string, error) {
	history, err := s.repo.FindByOldSlug(ctx, oldSlug)
	if err != nil {
		return "", "", fmt.Errorf("failed to search old slug: %w", err)
	}

	if history == nil {
		return "", "", nil // Not found
	}

	// Try to find the current slug
	currentSlug, err := s.repo.FindCurrentSlug(ctx, history.EntityType, history.EntityID, lang)
	if err != nil {
		return "", "", fmt.Errorf("failed to get current slug: %w", err)
	}

	return history.EntityType, currentSlug, nil
}
