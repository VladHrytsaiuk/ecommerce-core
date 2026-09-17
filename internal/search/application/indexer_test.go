package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	search "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
)

func TestProductChangedHandlerIndexesCurrentActiveSnapshot(t *testing.T) {
	productID := uuid.New()
	event, err := catalogDomain.NewProductChangedEvent(uuid.New(), productID, catalogDomain.ProductChangeUpsert, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := event.MarshalPayload()
	index := &fakeIndexer{}
	handler, err := NewProductChangedHandler(fakeSnapshots{document: &search.SearchDocument{ID: productID.String(), Status: "active"}}, index)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(context.Background(), events.Delivery{AggregateID: productID, Payload: payload}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(index.upserts) != 1 || index.upserts[0].ID != productID.String() || index.deleted != uuid.Nil {
		t.Fatalf("index operations = %+v / delete=%s", index.upserts, index.deleted)
	}
}

func TestProductChangedHandlerDeletesWhenDelayedUpsertFindsNoSnapshot(t *testing.T) {
	productID := uuid.New()
	event, _ := catalogDomain.NewProductChangedEvent(uuid.New(), productID, catalogDomain.ProductChangeUpsert, time.Now())
	payload, _ := event.MarshalPayload()
	index := &fakeIndexer{}
	handler, _ := NewProductChangedHandler(fakeSnapshots{err: search.ErrSnapshotNotFound}, index)
	if err := handler.Handle(context.Background(), events.Delivery{AggregateID: productID, Payload: payload}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if index.deleted != productID {
		t.Fatalf("deleted = %s, want %s", index.deleted, productID)
	}
}

func TestProductChangedHandlerRejectsAggregateMismatch(t *testing.T) {
	productID := uuid.New()
	event, _ := catalogDomain.NewProductChangedEvent(uuid.New(), productID, catalogDomain.ProductChangeUpsert, time.Now())
	payload, _ := event.MarshalPayload()
	handler, _ := NewProductChangedHandler(fakeSnapshots{}, &fakeIndexer{})
	if err := handler.Handle(context.Background(), events.Delivery{AggregateID: uuid.New(), Payload: payload}); err == nil {
		t.Fatal("Handle() error = nil, want invalid payload failure")
	}
}

type fakeSnapshots struct {
	document *search.SearchDocument
	err      error
}

func (f fakeSnapshots) Snapshot(context.Context, uuid.UUID) (*search.SearchDocument, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.document, nil
}

type fakeIndexer struct {
	upserts []search.SearchDocument
	deleted uuid.UUID
	err     error
}

func (f *fakeIndexer) AddOrUpdateDocuments(_ context.Context, documents ...search.SearchDocument) error {
	if f.err != nil {
		return f.err
	}
	f.upserts = append(f.upserts, documents...)
	return nil
}

func (f *fakeIndexer) DeleteDocument(_ context.Context, id uuid.UUID) error {
	if f.err != nil && !errors.Is(f.err, search.ErrSnapshotNotFound) {
		return f.err
	}
	f.deleted = id
	return nil
}
