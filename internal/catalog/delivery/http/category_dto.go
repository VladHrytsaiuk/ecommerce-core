package http

import (
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
)

type CatalogCategoryResponse struct {
	ID           uuid.UUID                     `json:"id"`
	ParentID     *uuid.UUID                    `json:"parent_id,omitempty"`
	SortOrder    int                           `json:"sort_order"`
	IsActive     bool                          `json:"is_active"`
	Translations []CategoryTranslationResponse `json:"translations"`
}

type CategoryTranslationResponse struct {
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Slug        string `json:"slug"`
}

func mapCategory(category *domain.Category) CatalogCategoryResponse {
	translations := make([]CategoryTranslationResponse, 0, len(category.Translations))
	for _, translation := range category.Translations {
		translations = append(translations, CategoryTranslationResponse{
			Locale: translation.Locale, Name: translation.Name, Description: translation.Description, Slug: translation.Slug,
		})
	}
	return CatalogCategoryResponse{ID: category.ID, ParentID: category.ParentID, SortOrder: category.SortOrder, IsActive: category.IsActive, Translations: translations}
}
