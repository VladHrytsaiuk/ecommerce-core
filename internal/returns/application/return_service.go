// Package application orchestrates RMA lifecycle changes without importing
// payment, inventory, HTTP, or PostgreSQL adapters.
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	returns "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
)

var (
	ErrReturnNotFound          = errors.New("return request not found")
	ErrReturnCustomerForbidden = errors.New("return request does not belong to customer")
	ErrUnsupportedRefundMode   = errors.New("automatic settlement supports full refunds only")
)

type CreateCommand struct {
	OrderID, CustomerID uuid.UUID
	RefundMode          returns.RefundMode
	Items               []returns.ReturnItem
}

type ReturnService struct {
	repository          returns.Repository
	orders              returns.OrderSnapshotReader
	eligibility         returns.ReturnEligibilityPolicy
	tx                  returns.TransactionManager
	statusPublisher     events.TransactionalEventPublisher
	settlementPublisher events.TransactionalEventPublisher
	now                 func() time.Time
}

func NewReturnService(repository returns.Repository, orders returns.OrderSnapshotReader, eligibility returns.ReturnEligibilityPolicy, tx returns.TransactionManager, statusPublisher, settlementPublisher events.TransactionalEventPublisher) (*ReturnService, error) {
	if repository == nil || orders == nil || eligibility == nil || tx == nil || statusPublisher == nil || settlementPublisher == nil {
		return nil, fmt.Errorf("returns service dependencies are required")
	}
	return &ReturnService{repository: repository, orders: orders, eligibility: eligibility, tx: tx, statusPublisher: statusPublisher, settlementPublisher: settlementPublisher, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *ReturnService) CreateReturnRequest(ctx context.Context, command CreateCommand) (*returns.ReturnRequest, error) {
	if s == nil || command.OrderID == uuid.Nil || command.CustomerID == uuid.Nil || len(command.Items) == 0 {
		return nil, fmt.Errorf("invalid return creation command")
	}
	if command.RefundMode == "" {
		command.RefundMode = returns.RefundModeFull
	}
	if command.RefundMode != returns.RefundModeFull {
		// There is no safe amount-allocation policy for partial refunds yet and
		// store credit requires its own ledger. Reject rather than creating an
		// RMA that would later be stranded in the settlement DLQ.
		return nil, ErrUnsupportedRefundMode
	}
	snapshot, err := s.orders.GetOrderSnapshot(ctx, command.OrderID)
	if err != nil {
		return nil, err
	}
	// Public requests use customer actor facts derived only from JWT in delivery.
	actorID := command.CustomerID
	request, err := returns.NewReturnRequest(command.OrderID, command.CustomerID, command.RefundMode, command.Items, returns.Actor{Type: returns.ActorTypeCustomer, ID: &actorID}, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.eligibility.IsEligible(snapshot, *request); err != nil {
		return nil, err
	}
	if err := validateRequestedItems(snapshot, request.Items); err != nil {
		return nil, err
	}
	if err := s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		return s.repository.Create(txCtx, request)
	}); err != nil {
		return nil, err
	}
	return request, nil
}

func (s *ReturnService) ApproveReturn(ctx context.Context, returnID, adminID uuid.UUID, reason string) (*returns.ReturnRequest, error) {
	if returnID == uuid.Nil || adminID == uuid.Nil {
		return nil, fmt.Errorf("invalid return approval command")
	}
	return s.transition(ctx, returnID, returns.Actor{Type: returns.ActorTypeAdmin, ID: &adminID}, reason, returns.ReturnStatusApproved)
}

// ReceiveReturn does not invoke provider I/O while its SQL transaction is
// open. It persists an immutable received transition plus a durable settlement
// command. The Outbox consumer subsequently restocks unopened lines and starts
// the provider refund with retries and a stable return-ID idempotency key.
func (s *ReturnService) ReceiveReturn(ctx context.Context, returnID, adminID uuid.UUID, reason string) (*returns.ReturnRequest, error) {
	if returnID == uuid.Nil || adminID == uuid.Nil {
		return nil, fmt.Errorf("invalid return receipt command")
	}
	var result *returns.ReturnRequest
	err := s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		request, err := s.repository.GetForUpdate(txCtx, returnID)
		if err != nil {
			return normalizeRepositoryError(err)
		}
		if request.Status == returns.ReturnStatusReceived {
			result = request
			return nil
		}
		from := request.Status
		history, err := request.Receive(returns.Actor{Type: returns.ActorTypeAdmin, ID: &adminID}, reason, s.now())
		if err != nil {
			return err
		}
		if err := s.repository.Update(txCtx, request, history); err != nil {
			return err
		}
		if err := s.publishStatusChanged(txCtx, request.ID, from, history); err != nil {
			return err
		}
		settlement, err := returns.NewSettlementRequestedEvent(history.ID, request.ID, request.OrderID, history.CreatedAt)
		if err != nil {
			return err
		}
		if err := s.settlementPublisher.Publish(txCtx, settlement); err != nil {
			return err
		}
		result = request
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ConfirmRefunded is called only by the verified payment-refund Outbox
// consumer. Gateway HTTP success is not financial confirmation.
func (s *ReturnService) ConfirmRefunded(ctx context.Context, orderID uuid.UUID) error {
	if orderID == uuid.Nil {
		return fmt.Errorf("invalid refund confirmation order")
	}
	return s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		request, err := s.repository.FindReceivedByOrderForUpdate(txCtx, orderID)
		if errors.Is(err, returns.ErrReturnRequestNotFound) {
			// A refund may be manual or unrelated to RMA; it is safe to ignore.
			return nil
		}
		if err != nil {
			return err
		}
		from := request.Status
		history, err := request.MarkRefunded(returns.Actor{Type: returns.ActorTypeSystem}, "verified payment refund", s.now())
		if err != nil {
			return err
		}
		if err := s.repository.Update(txCtx, request, history); err != nil {
			return err
		}
		return s.publishStatusChanged(txCtx, request.ID, from, history)
	})
}

func (s *ReturnService) transition(ctx context.Context, returnID uuid.UUID, actor returns.Actor, reason string, target returns.ReturnStatus) (*returns.ReturnRequest, error) {
	var result *returns.ReturnRequest
	err := s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		request, err := s.repository.GetForUpdate(txCtx, returnID)
		if err != nil {
			return normalizeRepositoryError(err)
		}
		if request.Status == target { // HTTP retry: no duplicate history/outbox event.
			result = request
			return nil
		}
		from := request.Status
		var history returns.ReturnStatusHistory
		switch target {
		case returns.ReturnStatusApproved:
			history, err = request.Approve(actor, reason, s.now())
		default:
			return fmt.Errorf("unsupported return transition")
		}
		if err != nil {
			return err
		}
		if err := s.repository.Update(txCtx, request, history); err != nil {
			return err
		}
		if err := s.publishStatusChanged(txCtx, request.ID, from, history); err != nil {
			return err
		}
		result = request
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *ReturnService) publishStatusChanged(ctx context.Context, returnID uuid.UUID, from returns.ReturnStatus, history returns.ReturnStatusHistory) error {
	event, err := returns.NewStatusChangedEvent(history.ID, returnID, from, history.Status, history.ActorType, history.CreatedAt)
	if err != nil {
		return err
	}
	return s.statusPublisher.Publish(ctx, event)
}

func validateRequestedItems(snapshot returns.OrderSnapshot, items []returns.ReturnItem) error {
	purchased := make(map[uuid.UUID]int, len(snapshot.Items))
	for _, item := range snapshot.Items {
		if item.VariantID != nil && *item.VariantID != uuid.Nil && item.Quantity > 0 {
			purchased[*item.VariantID] += item.Quantity
		}
	}
	for _, item := range items {
		if item.VariantID == uuid.Nil || item.Quantity <= 0 || purchased[item.VariantID] < item.Quantity {
			return fmt.Errorf("return item is not purchasable from this order")
		}
	}
	return nil
}

func normalizeRepositoryError(err error) error {
	if errors.Is(err, returns.ErrReturnRequestNotFound) {
		return err
	}
	return err
}
