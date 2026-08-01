package http

import (
	"github.com/google/uuid"
	categoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
)

func buildBreadcrumbs(cats []categoryDomain.Category, targetID uuid.UUID, lang string) []BreadcrumbResponse {
	// Build map for fast lookup
	catMap := make(map[uuid.UUID]categoryDomain.Category)
	for _, c := range cats {
		catMap[c.ID] = c
	}

	var path []BreadcrumbResponse
	currentID := targetID

	// Max depth to prevent infinite loops in case of corrupted data
	for i := 0; i < 20; i++ {
		cat, exists := catMap[currentID]
		if !exists {
			break
		}

		name := ""
		slug := ""
		for _, t := range cat.Translations {
			if t.LanguageCode == lang {
				name = t.Name
				slug = t.Slug
				break
			}
		}

		// Fallback to first translation if target language is missing
		if name == "" && len(cat.Translations) > 0 {
			name = cat.Translations[0].Name
			slug = cat.Translations[0].Slug
		}

		path = append([]BreadcrumbResponse{{
			ID:   cat.ID,
			Name: name,
			Slug: slug,
		}}, path...)

		if cat.ParentID == nil || *cat.ParentID == uuid.Nil {
			break
		}
		currentID = *cat.ParentID
	}

	return path
}
