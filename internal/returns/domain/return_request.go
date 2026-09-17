// Package domain contains the provider-neutral Returns / RMA lifecycle.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ReturnStatus string

const (
	ReturnStatusNew      ReturnStatus = "new"
	ReturnStatusApproved ReturnStatus = "approved"
	ReturnStatusRejected ReturnStatus = "rejected"
	ReturnStatusReceived ReturnStatus = "received"
	ReturnStatusRefunded ReturnStatus = "refunded"
	ReturnStatusClosed   ReturnStatus = "closed"
)

type RefundMode string

const (
	RefundModeFull        RefundMode = "full"
	RefundModePartial     RefundMode = "partial"
	RefundModeStoreCredit RefundMode = "store_credit"
)

type ItemCondition string

const (
	ItemConditionUnopened ItemCondition = "unopened"
	ItemConditionOpened   ItemCondition = "opened"
	ItemConditionDamaged  ItemCondition = "damaged"
)

type ActorType string

const (
	ActorTypeSystem   ActorType = "system"
	ActorTypeAdmin    ActorType = "admin"
	ActorTypeCustomer ActorType = "customer"
)

var (
	ErrInvalidReturnRequest    = errors.New("invalid return request")
	ErrInvalidReturnItem       = errors.New("invalid return item")
	ErrInvalidReturnActor      = errors.New("invalid return actor")
	ErrInvalidReturnTransition = errors.New("invalid return status transition")
	ErrReturnRequestNotFound   = errors.New("return request not found")
)

// ReturnRequest is an RMA aggregate. It deliberately stores only UUID
// integration identities for Orders, Identity and Catalog; it does not import
// models or repositories owned by those modules.
type ReturnRequest struct {
	ID         uuid.UUID
	OrderID    uuid.UUID
	CustomerID uuid.UUID
	Status     ReturnStatus
	RefundMode RefundMode
	Items      []ReturnItem
	History    []ReturnStatusHistory
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type ReturnItem struct {
	ID              uuid.UUID
	ReturnRequestID uuid.UUID
	VariantID       uuid.UUID
	Quantity        int
	Condition       ItemCondition
	Reason          string
	CreatedAt       time.Time
}

type ReturnStatusHistory struct {
	ID              uuid.UUID
	ReturnRequestID uuid.UUID
	Status          ReturnStatus
	ActorType       ActorType
	ActorID         *uuid.UUID
	Reason          string
	CreatedAt       time.Time
}

type Actor struct {
	Type ActorType
	ID   *uuid.UUID
}

func NewReturnRequest(orderID, customerID uuid.UUID, refundMode RefundMode, items []ReturnItem, actor Actor, now time.Time) (*ReturnRequest, error) {
	if orderID == uuid.Nil || customerID == uuid.Nil {
		return nil, fmt.Errorf("%w: order and customer IDs are required", ErrInvalidReturnRequest)
	}
	if !isValidRefundMode(refundMode) {
		return nil, fmt.Errorf("%w: unsupported refund mode %q", ErrInvalidReturnRequest, refundMode)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: at least one item is required", ErrInvalidReturnRequest)
	}
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	if now.IsZero() {
		return nil, fmt.Errorf("%w: creation time is required", ErrInvalidReturnRequest)
	}

	requestID := uuid.New()
	seenVariants := make(map[uuid.UUID]struct{}, len(items))
	for index := range items {
		if err := items[index].Validate(); err != nil {
			return nil, err
		}
		if _, exists := seenVariants[items[index].VariantID]; exists {
			return nil, fmt.Errorf("%w: duplicate variant %s", ErrInvalidReturnItem, items[index].VariantID)
		}
		seenVariants[items[index].VariantID] = struct{}{}
		if items[index].ID == uuid.Nil {
			items[index].ID = uuid.New()
		}
		items[index].ReturnRequestID = requestID
		items[index].CreatedAt = now.UTC()
	}

	request := &ReturnRequest{
		ID: requestID, OrderID: orderID, CustomerID: customerID,
		Status: ReturnStatusNew, RefundMode: refundMode, Items: items,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
	history, err := request.recordStatus(actor, "", now)
	if err != nil {
		return nil, err
	}
	request.History = append(request.History, history)
	return request, nil
}

func (i ReturnItem) Validate() error {
	if i.VariantID == uuid.Nil || i.Quantity <= 0 || !isValidItemCondition(i.Condition) {
		return fmt.Errorf("%w: variant, positive quantity and supported condition are required", ErrInvalidReturnItem)
	}
	return nil
}

func (r *ReturnRequest) Approve(actor Actor, reason string, now time.Time) (ReturnStatusHistory, error) {
	return r.transition(ReturnStatusApproved, actor, reason, now)
}

func (r *ReturnRequest) Reject(actor Actor, reason string, now time.Time) (ReturnStatusHistory, error) {
	return r.transition(ReturnStatusRejected, actor, reason, now)
}

func (r *ReturnRequest) Receive(actor Actor, reason string, now time.Time) (ReturnStatusHistory, error) {
	return r.transition(ReturnStatusReceived, actor, reason, now)
}

// MarkRefunded is intentionally only a state transition. A future application
// service must invoke a PaymentGateway refund before it commits this transition.
func (r *ReturnRequest) MarkRefunded(actor Actor, reason string, now time.Time) (ReturnStatusHistory, error) {
	return r.transition(ReturnStatusRefunded, actor, reason, now)
}

func (r *ReturnRequest) Close(actor Actor, reason string, now time.Time) (ReturnStatusHistory, error) {
	return r.transition(ReturnStatusClosed, actor, reason, now)
}

func (r *ReturnRequest) transition(target ReturnStatus, actor Actor, reason string, now time.Time) (ReturnStatusHistory, error) {
	if r == nil || r.ID == uuid.Nil || !isValidReturnStatus(r.Status) || now.IsZero() {
		return ReturnStatusHistory{}, fmt.Errorf("%w: malformed aggregate", ErrInvalidReturnRequest)
	}
	if !canTransition(r.Status, target) {
		return ReturnStatusHistory{}, fmt.Errorf("%w: %s -> %s", ErrInvalidReturnTransition, r.Status, target)
	}
	if err := validateActor(actor); err != nil {
		return ReturnStatusHistory{}, err
	}
	r.Status = target
	r.UpdatedAt = now.UTC()
	history, err := r.recordStatus(actor, reason, now)
	if err != nil {
		return ReturnStatusHistory{}, err
	}
	r.History = append(r.History, history)
	return history, nil
}

func (r *ReturnRequest) recordStatus(actor Actor, reason string, now time.Time) (ReturnStatusHistory, error) {
	if err := validateActor(actor); err != nil {
		return ReturnStatusHistory{}, err
	}
	return ReturnStatusHistory{
		ID: uuid.New(), ReturnRequestID: r.ID, Status: r.Status,
		ActorType: actor.Type, ActorID: actor.ID, Reason: strings.TrimSpace(reason), CreatedAt: now.UTC(),
	}, nil
}

func canTransition(from, to ReturnStatus) bool {
	switch from {
	case ReturnStatusNew:
		return to == ReturnStatusApproved || to == ReturnStatusRejected
	case ReturnStatusApproved:
		return to == ReturnStatusReceived || to == ReturnStatusRejected || to == ReturnStatusClosed
	case ReturnStatusReceived:
		return to == ReturnStatusRefunded || to == ReturnStatusClosed
	case ReturnStatusRefunded, ReturnStatusRejected:
		return to == ReturnStatusClosed
	default:
		return false
	}
}

func validateActor(actor Actor) error {
	switch actor.Type {
	case ActorTypeSystem:
		if actor.ID != nil {
			return fmt.Errorf("%w: system actor must not have an ID", ErrInvalidReturnActor)
		}
	case ActorTypeAdmin, ActorTypeCustomer:
		if actor.ID == nil || *actor.ID == uuid.Nil {
			return fmt.Errorf("%w: %s actor requires an ID", ErrInvalidReturnActor, actor.Type)
		}
	default:
		return fmt.Errorf("%w: unsupported actor type %q", ErrInvalidReturnActor, actor.Type)
	}
	return nil
}

func isValidReturnStatus(status ReturnStatus) bool {
	switch status {
	case ReturnStatusNew, ReturnStatusApproved, ReturnStatusRejected, ReturnStatusReceived, ReturnStatusRefunded, ReturnStatusClosed:
		return true
	default:
		return false
	}
}

func isValidRefundMode(mode RefundMode) bool {
	return mode == RefundModeFull || mode == RefundModePartial || mode == RefundModeStoreCredit
}

func isValidItemCondition(condition ItemCondition) bool {
	return condition == ItemConditionUnopened || condition == ItemConditionOpened || condition == ItemConditionDamaged
}
