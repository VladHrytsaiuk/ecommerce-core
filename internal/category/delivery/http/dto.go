package http

import (
	"time"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
)

// CategoryResponse DTO для видачі списку категорій
type CategoryResponse struct {
	ID              uuid.UUID         `json:"id"`
	ParentID        *uuid.UUID        `json:"parent_id"`
	Slug            string            `json:"slug"`
	Slugs           map[string]string `json:"slugs,omitempty"`
	Name            string            `json:"name"`
	MetaTitle       string            `json:"meta_title,omitempty"`
	MetaDescription string            `json:"meta_description,omitempty"`
	MetaKeywords    string            `json:"meta_keywords,omitempty"`
	IconURL         *string           `json:"icon_url"`
	SortOrder       int               `json:"sort_order"`
	CreatedAt       time.Time         `json:"created_at"`
}

// ErrorResponse стандартна структура помилки для API
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func mapCategoryToResponse(cat domain.Category, lang string) CategoryResponse {
	name := ""
	slug := ""
	slugsMap := make(map[string]string)
	metaTitle := ""
	metaDesc := ""
	metaKeywords := ""

	var currentLangTranslation *domain.CategoryTranslation
	for i, t := range cat.Translations {
		slugsMap[t.LanguageCode] = t.Slug
		if t.LanguageCode == lang {
			currentLangTranslation = &cat.Translations[i]
		}
	}
	if currentLangTranslation == nil && len(cat.Translations) > 0 {
		currentLangTranslation = &cat.Translations[0]
	}

	if currentLangTranslation != nil {
		name = currentLangTranslation.Name
		slug = currentLangTranslation.Slug
		metaTitle = currentLangTranslation.MetaTitle
		metaDesc = currentLangTranslation.MetaDescription
		metaKeywords = currentLangTranslation.MetaKeywords
	}

	return CategoryResponse{
		ID:              cat.ID,
		ParentID:        cat.ParentID,
		Slug:            slug,
		Slugs:           slugsMap,
		Name:            name,
		MetaTitle:       metaTitle,
		MetaDescription: metaDesc,
		MetaKeywords:    metaKeywords,
		IconURL:         cat.IconURL,
		SortOrder:       cat.SortOrder,
		CreatedAt:       cat.CreatedAt,
	}
}

func mapCategoryListToResponse(cats []domain.Category, lang string) []CategoryResponse {
	res := make([]CategoryResponse, len(cats))
	for i, c := range cats {
		res[i] = mapCategoryToResponse(c, lang)
	}
	return res
}

// --- Admin DTOs ---

type CreateCategoryRequest struct {
	ParentID  *uuid.UUID `json:"parent_id"`
	Slug      string     `json:"slug"`
	NameUk            string     `json:"name_uk" binding:"required"`
	NameEn            string     `json:"name_en"`
	MetaTitleUk       string     `json:"meta_title_uk"`
	MetaTitleEn       string     `json:"meta_title_en"`
	MetaDescriptionUk string     `json:"meta_description_uk"`
	MetaDescriptionEn string     `json:"meta_description_en"`
	MetaKeywordsUk    string     `json:"meta_keywords_uk"`
	MetaKeywordsEn    string     `json:"meta_keywords_en"`
	IconURL   *string    `json:"icon_url"`
	SortOrder *int       `json:"sort_order"`
}

type UpdateCategoryRequest struct {
	ParentID  *uuid.UUID `json:"parent_id"`
	Slug      *string    `json:"slug"`
	NameUk            *string    `json:"name_uk"`
	NameEn            *string    `json:"name_en"`
	MetaTitleUk       *string    `json:"meta_title_uk"`
	MetaTitleEn       *string    `json:"meta_title_en"`
	MetaDescriptionUk *string    `json:"meta_description_uk"`
	MetaDescriptionEn *string    `json:"meta_description_en"`
	MetaKeywordsUk    *string    `json:"meta_keywords_uk"`
	MetaKeywordsEn    *string    `json:"meta_keywords_en"`
	IconURL   *string    `json:"icon_url"`
	SortOrder *int       `json:"sort_order"`
}

type ReorderCategoriesRequest struct {
	IDs []uuid.UUID `json:"ids" binding:"required"`
}
