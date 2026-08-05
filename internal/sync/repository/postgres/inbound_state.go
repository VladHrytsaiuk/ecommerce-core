package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	syncDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/sync/domain"
)

type InboundStateStore struct{ db *gorm.DB }

func NewInboundStateStore(db *gorm.DB) *InboundStateStore { return &InboundStateStore{db: db} }

type inboundStateRecord struct {
	Source          string
	EntityType      string
	ExternalID      string
	Version         string
	SourceUpdatedAt *time.Time
	PayloadHash     string
	Status          string
	LockedAt        *time.Time
	LastError       *string
}

func (inboundStateRecord) TableName() string { return "sync_external_entity_state" }

func (s *InboundStateStore) ClaimStockChange(ctx context.Context, change syncDomain.StockChange, now time.Time, lease time.Duration) (bool, error) {
	claimed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state inboundStateRecord
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source = ? AND entity_type = ? AND external_id = ?", change.Source, "stock", change.ExternalID).First(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&inboundStateRecord{Source: change.Source, EntityType: "stock", ExternalID: change.ExternalID, Version: change.Version, SourceUpdatedAt: &change.SourceUpdatedAt, PayloadHash: change.PayloadHash, Status: "received", LockedAt: &now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 1 {
				claimed = true
				return nil
			}
			return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source = ? AND entity_type = ? AND external_id = ?", change.Source, "stock", change.ExternalID).First(&state).Error
		}
		if err != nil {
			return err
		}
		if state.Version == change.Version {
			if state.PayloadHash != change.PayloadHash {
				return syncDomain.ErrInboundVersionConflict
			}
			if state.Status == "applied" || (state.Status == "received" && state.LockedAt != nil && state.LockedAt.After(now.Add(-lease))) {
				return nil
			}
			claimed = true
			return tx.Model(&inboundStateRecord{}).Where("source = ? AND entity_type = ? AND external_id = ?", change.Source, "stock", change.ExternalID).Updates(map[string]any{"status": "received", "locked_at": now, "last_error": nil, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error
		}
		if state.SourceUpdatedAt != nil && !change.SourceUpdatedAt.After(*state.SourceUpdatedAt) {
			return nil
		}
		claimed = true
		return tx.Model(&inboundStateRecord{}).Where("source = ? AND entity_type = ? AND external_id = ?", change.Source, "stock", change.ExternalID).Updates(map[string]any{"version": change.Version, "source_updated_at": change.SourceUpdatedAt, "payload_hash": change.PayloadHash, "status": "received", "locked_at": now, "last_error": nil, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error
	})
	if err != nil {
		return false, err
	}
	return claimed, nil
}

func (s *InboundStateStore) MarkStockChangeApplied(ctx context.Context, change syncDomain.StockChange) error {
	return s.updateStatus(ctx, change, "applied", nil)
}

func (s *InboundStateStore) MarkStockChangeFailed(ctx context.Context, change syncDomain.StockChange, cause error) error {
	if cause == nil {
		return fmt.Errorf("sync inbound failure requires a cause")
	}
	return s.updateStatus(ctx, change, "failed", cause)
}

func (s *InboundStateStore) updateStatus(ctx context.Context, change syncDomain.StockChange, status string, cause error) error {
	values := map[string]any{"status": status, "locked_at": nil, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}
	if cause != nil {
		values["last_error"] = cause.Error()
	} else {
		values["last_error"] = nil
	}
	result := s.db.WithContext(ctx).Model(&inboundStateRecord{}).Where("source = ? AND entity_type = ? AND external_id = ? AND version = ? AND payload_hash = ? AND status = ?", change.Source, "stock", change.ExternalID, change.Version, change.PayloadHash, "received").Updates(values)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("sync stock change %s/%s is not claimed", change.Source, change.ExternalID)
	}
	return nil
}

var _ syncDomain.InboundStateStore = (*InboundStateStore)(nil)
