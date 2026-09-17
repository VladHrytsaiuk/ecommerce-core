package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	search "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
)

// ProductChangedHandler converges the external index to the current catalog
// state. It is safe under at-least-once and out-of-order Outbox delivery.
type ProductChangedHandler struct {
	snapshots search.CatalogSnapshotProvider
	index     search.DocumentIndexer
}

func NewProductChangedHandler(snapshots search.CatalogSnapshotProvider, index search.DocumentIndexer) (*ProductChangedHandler, error) {
	if snapshots == nil || index == nil {
		return nil, fmt.Errorf("search snapshot provider and indexer are required")
	}
	return &ProductChangedHandler{snapshots: snapshots, index: index}, nil
}

func (*ProductChangedHandler) Topic() string { return domain.TopicProductChanged }

func (h *ProductChangedHandler) Handle(ctx context.Context, delivery events.Delivery) error {
	var payload struct {
		Version   int                           `json:"version"`
		ProductID uuid.UUID                     `json:"product_id"`
		Operation domain.ProductChangeOperation `json:"operation"`
	}
	if err := json.Unmarshal(delivery.Payload, &payload); err != nil {
		return fmt.Errorf("decode catalog product changed event: %w", err)
	}
	if payload.Version != 1 || payload.ProductID == uuid.Nil || payload.ProductID != delivery.AggregateID {
		return fmt.Errorf("invalid catalog product changed event")
	}
	if payload.Operation == domain.ProductChangeDelete {
		return h.index.DeleteDocument(ctx, payload.ProductID)
	}
	if payload.Operation != domain.ProductChangeUpsert {
		return fmt.Errorf("unsupported catalog product event operation %q", payload.Operation)
	}
	document, err := h.snapshots.Snapshot(ctx, payload.ProductID)
	if errors.Is(err, search.ErrSnapshotNotFound) {
		// A delete can race an older upsert event. Deleting is the desired
		// convergent state rather than a retriable worker failure.
		return h.index.DeleteDocument(ctx, payload.ProductID)
	}
	if err != nil {
		return err
	}
	if document == nil || document.ID != payload.ProductID.String() {
		return fmt.Errorf("invalid catalog search snapshot")
	}
	if document.Status != "active" {
		return h.index.DeleteDocument(ctx, payload.ProductID)
	}
	return h.index.AddOrUpdateDocuments(ctx, *document)
}

var _ interface {
	Topic() string
	Handle(context.Context, events.Delivery) error
} = (*ProductChangedHandler)(nil)
