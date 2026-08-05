package application

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	syncDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/sync/domain"
)

var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// ImportService is invoked only by an authenticated ERP delivery adapter. It
// owns idempotency and never accepts direct SQL or a browser stock mutation.
type ImportService struct {
	states syncDomain.InboundStateStore
	stock  domain.ExternalStockImporter
	lease  time.Duration
}

func NewImportService(states syncDomain.InboundStateStore, stock domain.ExternalStockImporter, lease time.Duration) *ImportService {
	if lease <= 0 {
		lease = time.Minute
	}
	return &ImportService{states: states, stock: stock, lease: lease}
}

func (s *ImportService) ApplyStockChange(ctx context.Context, change syncDomain.StockChange) error {
	if s.states == nil || s.stock == nil {
		return fmt.Errorf("sync stock importer is not configured")
	}
	if !validStockChange(change) {
		return fmt.Errorf("invalid sync stock change")
	}
	claimed, err := s.states.ClaimStockChange(ctx, change, time.Now().UTC(), s.lease)
	if err != nil || !claimed {
		return err
	}
	if err := s.stock.ReplaceExternalQuantity(ctx, change.VariantID, change.WarehouseID, change.Quantity); err != nil {
		_ = s.states.MarkStockChangeFailed(context.WithoutCancel(ctx), change, err)
		return err
	}
	return s.states.MarkStockChangeApplied(context.WithoutCancel(ctx), change)
}

func validStockChange(change syncDomain.StockChange) bool {
	return strings.TrimSpace(change.Source) != "" && strings.TrimSpace(change.ExternalID) != "" &&
		strings.TrimSpace(change.Version) != "" && !change.SourceUpdatedAt.IsZero() &&
		sha256Pattern.MatchString(strings.ToLower(change.PayloadHash)) && change.VariantID != uuid.Nil &&
		change.WarehouseID != uuid.Nil && change.Quantity >= 0
}
