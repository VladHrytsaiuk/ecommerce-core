package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type UrlHistory struct {
	ID         uuid.UUID `json:"id"`
	EntityType string    `json:"entity_type"`
	EntityID   uuid.UUID `json:"entity_id"`
	OldSlug    string    `json:"old_slug"`
	CreatedAt  time.Time `json:"created_at"`
}

func (UrlHistory) TableName() string {
	return "url_history"
}

type RedirectRepository interface {
	SaveHistory(ctx context.Context, history *UrlHistory) error
	FindByOldSlug(ctx context.Context, oldSlug string) (*UrlHistory, error)
	FindCurrentSlug(ctx context.Context, entityType string, entityID uuid.UUID, lang string) (string, error)
}

type RedirectService interface {
	// RecordSlugChange records the old slug for a given entity type and ID.
	RecordSlugChange(ctx context.Context, entityType string, entityID uuid.UUID, oldSlug string) error
	
	// ResolveRedirect attempts to find an active redirect for the given slug.
	// Returns the entity_type, the new_slug of the entity, and potentially an error.
	ResolveRedirect(ctx context.Context, oldSlug string, lang string) (entityType string, newSlug string, err error)
}
