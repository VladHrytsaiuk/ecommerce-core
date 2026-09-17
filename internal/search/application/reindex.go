package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	search "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
)

type Reindexer struct {
	source search.ActiveDocumentLister
	index  search.DocumentIndexer
}

func NewReindexer(source search.ActiveDocumentLister, index search.DocumentIndexer) (*Reindexer, error) {
	if source == nil || index == nil {
		return nil, fmt.Errorf("search reindex source and index are required")
	}
	return &Reindexer{source: source, index: index}, nil
}

func (r *Reindexer) Run(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 || batchSize > 1000 {
		return 0, fmt.Errorf("reindex batch size must be between 1 and 1000")
	}
	indexed := 0
	var after *uuid.UUID
	for {
		documents, next, err := r.source.ListActiveDocuments(ctx, after, batchSize)
		if err != nil {
			return indexed, err
		}
		if len(documents) == 0 {
			return indexed, nil
		}
		if err := r.index.AddOrUpdateDocuments(ctx, documents...); err != nil {
			return indexed, err
		}
		indexed += len(documents)
		if next == nil || *next == uuid.Nil {
			return indexed, fmt.Errorf("search reindex source returned documents without a cursor")
		}
		after = next
	}
}
