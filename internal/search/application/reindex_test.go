package application

import (
	"context"
	"testing"

	"github.com/google/uuid"

	search "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
)

func TestReindexerUsesBoundedBatches(t *testing.T) {
	first, second, third := uuid.New(), uuid.New(), uuid.New()
	source := &documentSourceStub{pages: []documentPage{
		{documents: []search.SearchDocument{{ID: first.String()}, {ID: second.String()}}, next: second},
		{documents: []search.SearchDocument{{ID: third.String()}}, next: third},
	}}
	index := &documentIndexStub{}
	reindexer, err := NewReindexer(source, index)
	if err != nil {
		t.Fatalf("NewReindexer() error = %v", err)
	}
	count, err := reindexer.Run(context.Background(), 2)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if count != 3 || len(index.batches) != 2 || len(index.batches[0]) != 2 || len(index.batches[1]) != 1 || len(source.cursors) != 3 || source.cursors[0] != uuid.Nil || source.cursors[1] != second || source.cursors[2] != third {
		t.Fatalf("reindex result count=%d batches=%#v", count, index.batches)
	}
}

type documentPage struct {
	documents []search.SearchDocument
	next      uuid.UUID
}

type documentSourceStub struct {
	pages   []documentPage
	cursors []uuid.UUID
}

func (s *documentSourceStub) ListActiveDocuments(_ context.Context, after *uuid.UUID, _ int) ([]search.SearchDocument, *uuid.UUID, error) {
	if after == nil {
		s.cursors = append(s.cursors, uuid.Nil)
	} else {
		s.cursors = append(s.cursors, *after)
	}
	page := len(s.cursors)
	if page > len(s.pages) {
		return nil, nil, nil
	}
	current := s.pages[page-1]
	next := current.next
	return current.documents, &next, nil
}

type documentIndexStub struct{ batches [][]search.SearchDocument }

func (s *documentIndexStub) AddOrUpdateDocuments(_ context.Context, documents ...search.SearchDocument) error {
	s.batches = append(s.batches, append([]search.SearchDocument(nil), documents...))
	return nil
}
func (*documentIndexStub) DeleteDocument(context.Context, uuid.UUID) error { return nil }
