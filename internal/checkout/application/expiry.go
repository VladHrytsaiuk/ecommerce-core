package application

import (
	"context"
	"fmt"
	"time"

	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/worker"
)

// ExpiryService closes checkout attempts that reached their persisted deadline.
// Each claimed order is completed by OrderWorkflow in its own atomic database
// transaction, including inventory and optional module hooks.
type ExpiryService struct {
	workflow workflowDomain.Service
	logger   worker.Logger
}

func NewExpiryService(workflow workflowDomain.Service) *ExpiryService {
	return &ExpiryService{workflow: workflow}
}

// WithLogger reports a failing pass. Expiry that stops running leaves stock
// reserved against checkouts nobody is going to complete, which shows up as
// phantom out-of-stock rather than as an error.
func (s *ExpiryService) WithLogger(logger worker.Logger) *ExpiryService {
	if s != nil && logger != nil {
		s.logger = logger
	}
	return s
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
	worker.Loop(ctx, interval, time.Minute, s.logger, "checkout expiry", func(ctx context.Context) error {
		_, err := s.ExpireOnce(ctx, 100)
		return err
	})
}
