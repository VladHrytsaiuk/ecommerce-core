//go:build legacy
// +build legacy

package service

import (
	"context"
	"encoding/json"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/document/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"github.com/google/uuid"
	slugLib "github.com/gosimple/slug"
)

type documentService struct {
	repo domain.DocumentRepository
	l    logger.Logger
}

func NewDocumentService(repo domain.DocumentRepository, l logger.Logger) domain.DocumentService {
	return &documentService{repo: repo, l: l}
}

func (s *documentService) GetBySlug(ctx context.Context, slug string) (*domain.Document, error) {
	return s.repo.FindBySlug(ctx, slug)
}

func (s *documentService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Document, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *documentService) GetList(ctx context.Context, pgn pagination.Params) ([]domain.Document, pagination.Metadata, error) {
	docs, total, err := s.repo.FindAll(ctx, pgn)
	if err != nil {
		return nil, pagination.Metadata{}, err
	}

	meta := pagination.CalculateMetadata(total, pgn.Page, pgn.Limit)
	return docs, meta, nil
}

func (s *documentService) Create(ctx context.Context, doc *domain.Document, initialContent json.RawMessage, changelog string, createdBy uuid.UUID) error {
	return s.repo.Atomic(ctx, func(repo domain.DocumentRepository) error {
		if doc.Slug == "" {
			var name string
			if ukName, ok := doc.Title["uk"]; ok && ukName != "" {
				name = ukName
			} else if enName, ok := doc.Title["en"]; ok && enName != "" {
				name = enName
			} else {
				for _, val := range doc.Title {
					if val != "" {
						name = val
						break
					}
				}
			}
			if name == "" {
				name = "document"
			}

			baseSlug := slugLib.Make(name)
			if baseSlug == "" {
				baseSlug = "document"
			}

			candidate := baseSlug
			// Check uniqueness up to 10 attempts
			for i := 1; i <= 10; i++ {
				_, err := repo.FindBySlug(ctx, candidate)
				if err != nil {
					// If document not found, candidate is unique
					doc.Slug = candidate
					break
				}
				// collision: append a short suffix
				suffix := uuid.New().String()[:4]
				candidate = baseSlug + "-" + suffix
			}
		}

		// 1. Create document with no current_version_id
		if err := repo.Create(ctx, doc); err != nil {
			return err
		}

		// 2. Create the first version
		version := &domain.DocumentVersion{
			DocumentID:    doc.ID,
			VersionNumber: 1,
			Content:       initialContent,
			CreatedBy:     createdBy,
		}
		if changelog != "" {
			version.Changelog = &changelog
		}

		if err := repo.CreateVersion(ctx, version); err != nil {
			return err
		}

		// 3. Update document with current_version_id
		doc.CurrentVersionID = &version.ID
		if err := repo.Update(ctx, doc); err != nil {
			return err
		}

		return nil
	})
}

func (s *documentService) SaveVersion(ctx context.Context, documentID uuid.UUID, content json.RawMessage, changelog string, createdBy uuid.UUID) error {
	return s.repo.Atomic(ctx, func(repo domain.DocumentRepository) error {
		doc, err := repo.FindByID(ctx, documentID)
		if err != nil {
			return err
		}

		nextVersionNumber, err := repo.GetNextVersionNumber(ctx, documentID)
		if err != nil {
			return err
		}

		version := &domain.DocumentVersion{
			DocumentID:    documentID,
			VersionNumber: nextVersionNumber,
			Content:       content,
			CreatedBy:     createdBy,
		}
		if changelog != "" {
			version.Changelog = &changelog
		}

		if err := repo.CreateVersion(ctx, version); err != nil {
			return err
		}

		doc.CurrentVersionID = &version.ID
		if err := repo.Update(ctx, doc); err != nil {
			return err
		}

		return nil
	})
}

func (s *documentService) SetActiveVersion(ctx context.Context, documentID uuid.UUID, versionID uuid.UUID) error {
	return s.repo.Atomic(ctx, func(repo domain.DocumentRepository) error {
		doc, err := repo.FindByID(ctx, documentID)
		if err != nil {
			return err
		}

		_, err = repo.FindVersionByID(ctx, versionID)
		if err != nil {
			return err
		}

		doc.CurrentVersionID = &versionID
		if err := repo.Update(ctx, doc); err != nil {
			return err
		}

		return nil
	})
}

func (s *documentService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
