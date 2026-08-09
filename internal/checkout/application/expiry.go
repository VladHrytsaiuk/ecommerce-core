package application

import (
	"context"
	"fmt"
	"time"

	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
)

// ExpiryService closes checkout attempts that reached their persisted deadline.
// Each claimed order is completed by OrderWorkflow in its own atomic database
// transaction, including inventory and optional module hooks.
type ExpiryService struct{ workflow workflowDomain.Service }

func NewExpiryService(workflow workflowDomain.Service) *ExpiryService {
	return &ExpiryService{workflow: workflow}
}
func (s *ExpiryService) ExpireOnce(ctx context.Context, limit int) (int, error) {
	if s.workflow == nil {
		return 0, fmt.Errorf("checkout expiry workflow is not configured")
	}
	if limit <= 0 {
		limit = 100
	}
	count := 0
	for count < limit {
		expired, err := s.workflow.ExpirePendingCheckout(ctx, time.Now().UTC())
		if err != nil || !expired {
			return count, err
		}
		count++
	}
	return count, nil
}
func (s *ExpiryService) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	for {
		_, _ = s.ExpireOnce(ctx, 100)
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}
