//go:build legacy && ignore
// +build legacy,ignore

package http

import (
	"encoding/json"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/document/domain"
	"github.com/google/uuid"
)

type CreateDocumentRequest struct {
	Slug      string                     `json:"slug"`
	Title     map[string]string          `json:"title" binding:"required"`
	Content   map[string]json.RawMessage `json:"content" binding:"required" swaggertype:"object"`
	Changelog string                     `json:"changelog"`
}

type SaveVersionRequest struct {
	Content   map[string]json.RawMessage `json:"content" binding:"required" swaggertype:"object"`
	Changelog string                     `json:"changelog"`
}

type SetActiveVersionRequest struct {
	VersionID uuid.UUID `json:"version_id" binding:"required"`
}

type PublicDocumentResponse struct {
	ID            uuid.UUID       `json:"id"`
	Slug          string          `json:"slug"`
	Title         string          `json:"title"`
	Content       json.RawMessage `json:"content" swaggertype:"object"`
	VersionNumber int             `json:"version_number"`
	UpdatedAt     time.Time       `json:"updated_at"`
	IsFallback    bool            `json:"is_fallback,omitempty"`
}

type DocumentListResponse struct {
	ID               uuid.UUID         `json:"id"`
	Slug             string            `json:"slug"`
	Title            map[string]string `json:"title"`
	CurrentVersionID *uuid.UUID        `json:"current_version_id"`
	VersionNumber    int               `json:"version_number,omitempty"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type DocumentVersionResponse struct {
	ID            uuid.UUID       `json:"id"`
	VersionNumber int             `json:"version_number"`
	Content       json.RawMessage `json:"content" swaggertype:"object"`
	CreatedBy     uuid.UUID       `json:"created_by"`
	CreatedByName string          `json:"created_by_name,omitempty"`
	Changelog     *string         `json:"changelog"`
	CreatedAt     time.Time       `json:"created_at"`
}

type AdminDocumentDetailResponse struct {
	ID             uuid.UUID                 `json:"id"`
	Slug           string                    `json:"slug"`
	Title          map[string]string         `json:"title"`
	CurrentVersion *DocumentVersionResponse  `json:"current_version,omitempty"`
	Versions       []DocumentVersionResponse `json:"versions"`
	CreatedAt      time.Time                 `json:"created_at"`
	UpdatedAt      time.Time                 `json:"updated_at"`
}

// Mappers

func mapPublicDocument(doc domain.Document, lang string) PublicDocumentResponse {
	title := doc.Title[lang]
	if title == "" {
		title = doc.Title["uk"] // fallback
	}

	res := PublicDocumentResponse{
		ID:        doc.ID,
		Slug:      doc.Slug,
		Title:     title,
		UpdatedAt: doc.UpdatedAt,
	}

	if doc.CurrentVersion != nil {
		res.VersionNumber = doc.CurrentVersion.VersionNumber

		var contentMap map[string]json.RawMessage
		if err := json.Unmarshal(doc.CurrentVersion.Content, &contentMap); err == nil {
			content, exists := contentMap[lang]
			if exists && len(content) > 0 && string(content) != "null" {
				res.Content = content
			} else {
				// Fallback to uk
				fallbackContent, fallbackExists := contentMap["uk"]
				if fallbackExists {
					res.Content = fallbackContent
					res.IsFallback = true
				} else {
					res.Content = json.RawMessage("{}") // Empty object fallback
				}
			}
		} else {
			res.Content = json.RawMessage("{}")
		}
	} else {
		res.Content = json.RawMessage("{}")
	}

	return res
}

func mapDocumentList(doc domain.Document) DocumentListResponse {
	res := DocumentListResponse{
		ID:               doc.ID,
		Slug:             doc.Slug,
		Title:            doc.Title,
		CurrentVersionID: doc.CurrentVersionID,
		UpdatedAt:        doc.UpdatedAt,
	}
	if doc.CurrentVersion != nil {
		res.VersionNumber = doc.CurrentVersion.VersionNumber
	}
	return res
}

func mapDocumentVersion(version domain.DocumentVersion) DocumentVersionResponse {
	res := DocumentVersionResponse{
		ID:            version.ID,
		VersionNumber: version.VersionNumber,
		Content:       version.Content,
		CreatedBy:     version.CreatedBy,
		Changelog:     version.Changelog,
		CreatedAt:     version.CreatedAt,
	}
	if version.CreatedByUser != nil {
		res.CreatedByName = version.CreatedByUser.FirstName + " " + version.CreatedByUser.LastName
	}
	return res
}

func mapAdminDocumentDetail(doc domain.Document) AdminDocumentDetailResponse {
	res := AdminDocumentDetailResponse{
		ID:        doc.ID,
		Slug:      doc.Slug,
		Title:     doc.Title,
		CreatedAt: doc.CreatedAt,
		UpdatedAt: doc.UpdatedAt,
		Versions:  make([]DocumentVersionResponse, 0, len(doc.Versions)),
	}

	if doc.CurrentVersion != nil {
		cv := mapDocumentVersion(*doc.CurrentVersion)
		res.CurrentVersion = &cv
	}

	for _, v := range doc.Versions {
		res.Versions = append(res.Versions, mapDocumentVersion(v))
	}

	return res
}
