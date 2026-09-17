package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const TopicProductChanged = "catalog.product.changed.v1"

type ProductChangeOperation string

const (
	ProductChangeUpsert ProductChangeOperation = "upsert"
	ProductChangeDelete ProductChangeOperation = "delete"
)

// ProductChangedEvent contains no catalog payload. Consumers must read the
// current snapshot by ProductID, which makes delayed or repeated deliveries
// converge to the authoritative PostgreSQL state.
type ProductChangedEvent struct {
	ProductID uuid.UUID              `json:"product_id"`
	Operation ProductChangeOperation `json:"operation"`
	ChangeID  uuid.UUID              `json:"change_id"`
	ChangedAt time.Time              `json:"changed_at"`
}

func NewProductChangedEvent(changeID, productID uuid.UUID, operation ProductChangeOperation, changedAt time.Time) (ProductChangedEvent, error) {
	if changeID == uuid.Nil || productID == uuid.Nil {
		return ProductChangedEvent{}, fmt.Errorf("catalog product change requires change and product IDs")
	}
	if operation != ProductChangeUpsert && operation != ProductChangeDelete {
		return ProductChangedEvent{}, fmt.Errorf("unsupported catalog product change operation %q", operation)
	}
	if changedAt.IsZero() {
		changedAt = time.Now().UTC()
	}
	return ProductChangedEvent{ProductID: productID, Operation: operation, ChangeID: changeID, ChangedAt: changedAt.UTC()}, nil
}

func (ProductChangedEvent) Topic() string               { return TopicProductChanged }
func (ProductChangedEvent) AggregateType() string       { return "product" }
func (e ProductChangedEvent) AggregateID() uuid.UUID    { return e.ProductID }
func (e ProductChangedEvent) IdempotencyKey() uuid.UUID { return e.ChangeID }
func (e ProductChangedEvent) OccurredAt() time.Time     { return e.ChangedAt }

func (e ProductChangedEvent) MarshalPayload() ([]byte, error) {
	if strings.TrimSpace(string(e.Operation)) == "" {
		return nil, fmt.Errorf("catalog product change operation is required")
	}
	return json.Marshal(struct {
		Version   int                    `json:"version"`
		ProductID uuid.UUID              `json:"product_id"`
		Operation ProductChangeOperation `json:"operation"`
		ChangedAt time.Time              `json:"changed_at"`
	}{Version: 1, ProductID: e.ProductID, Operation: e.Operation, ChangedAt: e.ChangedAt})
}
