package projectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	reports "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
)

type FunnelProjector struct {
	repository reports.ReportsRepository
	tx         reports.TransactionManager
	topic      string
	location   *time.Location
}

func NewFunnelProjector(repository reports.ReportsRepository, tx reports.TransactionManager, topic, timezone string) (*FunnelProjector, error) {
	if repository == nil || tx == nil || (topic != events.TopicCartCreated && topic != events.TopicCheckoutStarted) {
		return nil, fmt.Errorf("invalid funnel projector dependencies")
	}
	if strings.TrimSpace(timezone) == "" {
		timezone = "UTC"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load reports timezone: %w", err)
	}
	return &FunnelProjector{repository: repository, tx: tx, topic: topic, location: location}, nil
}
func (p *FunnelProjector) Topic() string { return p.topic }
func (p *FunnelProjector) Handle(ctx context.Context, delivery events.Delivery) error {
	if delivery.EventID == uuid.Nil || delivery.AggregateID == uuid.Nil || delivery.Topic != p.topic {
		return fmt.Errorf("invalid funnel delivery")
	}
	var payload struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(delivery.Payload, &payload); err != nil || payload.Version != 1 {
		return fmt.Errorf("invalid funnel event payload")
	}
	local := delivery.OccurredAt.In(p.location)
	if delivery.OccurredAt.IsZero() {
		return fmt.Errorf("funnel event occurrence is required")
	}
	funnel := reports.DailyFunnel{BucketDate: time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, p.location), Channel: defaultChannel}
	if p.topic == events.TopicCartCreated {
		funnel.CartsCreated = 1
	} else {
		funnel.CheckoutsStarted = 1
	}
	return p.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := p.repository.AcquireProjectionLock(txCtx); err != nil {
			return err
		}
		inserted, err := p.repository.MarkEventProcessed(txCtx, delivery.EventID, delivery.Topic)
		if err != nil || !inserted {
			return err
		}
		return p.repository.UpsertDailyFunnel(txCtx, funnel)
	})
}

var _ interface {
	Topic() string
	Handle(context.Context, events.Delivery) error
} = (*FunnelProjector)(nil)
