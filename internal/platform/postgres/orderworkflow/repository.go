// Package orderworkflow contains the PostgreSQL transaction adapter for the
// commerce workflow. It is infrastructure, not a core or module repository:
// it is the one explicit place permitted to coordinate persisted Order and
// Inventory state in one database transaction.
package orderworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	eventsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	transaction "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

type Repository struct {
	db          *gorm.DB
	syncEnabled bool
	hooks       []workflowDomain.TransactionHook
	events      eventsDomain.TransactionalEventPublisher
}

// NewRepository constructs the one cross-context PostgreSQL transaction
// adapter. syncEnabled is derived exclusively from ENABLED_MODULES in
// Bootstrap, so deployments without the Sync module never touch its tables.
func NewRepository(db *gorm.DB, syncEnabled bool) *Repository {
	return &Repository{db: db, syncEnabled: syncEnabled}
}

// WithTransactionHook registers an optional module hook. It is called only
// from the workflow's database transactions, never around provider I/O.
func (r *Repository) WithTransactionHook(hook workflowDomain.TransactionHook) *Repository {
	if hook != nil {
		r.hooks = append(r.hooks, hook)
	}
	return r
}

// WithEventPublisher enables durable domain events within the workflow's
// existing database transaction. The publisher must never perform network I/O.
func (r *Repository) WithEventPublisher(publisher eventsDomain.TransactionalEventPublisher) *Repository {
	r.events = publisher
	return r
}

type reservationRecord struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	VariantID   uuid.UUID
	WarehouseID uuid.UUID
	Quantity    int
	Status      string
	ExpiresAt   time.Time
	OrderID     *uuid.UUID
}

func (reservationRecord) TableName() string { return "inventory_reservations" }

type orderStateRecord struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey"`
	CartID           *uuid.UUID
	Number           string
	Status           string
	Currency         string
	TotalAmount      int64
	PaymentProvider  string
	DeliveryProvider string
	ExpiresAt        time.Time
}

func (orderStateRecord) TableName() string { return "orders" }

type orderRecord struct {
	ID                    uuid.UUID `gorm:"type:uuid;primaryKey"`
	CartID                *uuid.UUID
	Number                string
	CustomerID            *uuid.UUID
	Status                string
	Currency              string
	SubtotalAmount        int64
	TaxAmount             int64
	ShippingAmount        int64
	TotalAmount           int64
	PaymentProvider       string
	DeliveryProvider      string
	AppliedPromoCode      *string
	DiscountAmount        int64
	PromoSnapshotType     *string
	PromoSnapshotValue    *int64
	PromoSnapshotCurrency *string
	ExpiresAt             time.Time
}

func (orderRecord) TableName() string { return "orders" }

type itemRecord struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrderID         uuid.UUID
	VariantID       *uuid.UUID
	ProductName     string
	SKU             string
	Quantity        int
	UnitPriceAmount int64
	TotalAmount     int64
	Currency        string
	UnitWeightGrams int
}

func (itemRecord) TableName() string { return "order_items" }

type deliveryDetailsRecord struct {
	OrderID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	RecipientName  string
	RecipientPhone string
	CountryCode    string
	PostalCode     string
	City           string
	Line1          string
	Line2          string
	LocalityID     string
	ServicePointID string
}

func (deliveryDetailsRecord) TableName() string { return "order_delivery_details" }

type contactDetailsRecord struct {
	OrderID uuid.UUID `gorm:"type:uuid;primaryKey"`
	Email   string
	Locale  string
}

func (contactDetailsRecord) TableName() string { return "order_contact_details" }

type deliveryJobRecord struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrderID        uuid.UUID
	Provider       string
	IdempotencyKey uuid.UUID
	Status         string
}

type syncOutboxRecord struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	Topic          string
	AggregateID    uuid.UUID
	IdempotencyKey uuid.UUID
	Payload        string
	Status         string
}

func (syncOutboxRecord) TableName() string { return "sync_outbox" }

type paymentRecord struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrderID           uuid.UUID
	Provider          string
	ProviderReference string
	Status            string
	Amount            int64
	Currency          string
}

func (paymentRecord) TableName() string { return "payments" }

type paymentCheckoutAttemptRecord struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrderID           uuid.UUID
	Provider          string
	IdempotencyKey    string
	Amount            int64
	Currency          string
	ProviderReference *string
	Status            string
	Attempts          int
	LastError         *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LockedAt          *time.Time
	ExpiresAt         time.Time
}

type paymentAnomalyRecord struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrderID           uuid.UUID
	Provider          string
	ProviderReference string
	Amount            int64
	Currency          string
	Reason            string
	Status            string
}

func (paymentAnomalyRecord) TableName() string { return "payment_anomalies" }

func (paymentCheckoutAttemptRecord) TableName() string { return "payment_checkout_attempts" }

func (deliveryJobRecord) TableName() string { return "delivery_jobs" }

func (r *Repository) CreatePending(ctx context.Context, order *ordersDomain.Order, reservationIDs []uuid.UUID) error {
	return r.createPending(ctx, order, reservationIDs, nil, false)
}

func (r *Repository) CreatePendingCheckout(ctx context.Context, order *ordersDomain.Order, reservationIDs []uuid.UUID, attempt workflowDomain.CheckoutAttemptRequest) error {
	return r.createPending(ctx, order, reservationIDs, &attempt, false)
}

func (r *Repository) CreatePaidCheckout(ctx context.Context, order *ordersDomain.Order, reservationIDs []uuid.UUID, attempt workflowDomain.CheckoutAttemptRequest) error {
	return r.createPending(ctx, order, reservationIDs, &attempt, true)
}

func (r *Repository) createPending(ctx context.Context, order *ordersDomain.Order, reservationIDs []uuid.UUID, attempt *workflowDomain.CheckoutAttemptRequest, markPaid bool) error {
	return r.withTransaction(ctx, func(tx *gorm.DB) error {
		if order.ExpiresAt.IsZero() {
			order.ExpiresAt = time.Now().UTC().Add(30 * time.Minute)
		}
		reservations, err := lockReservations(tx, reservationIDs)
		if err != nil {
			return err
		}
		for _, reservation := range reservations {
			if reservation.Status != "active" || reservation.OrderID != nil || !reservation.ExpiresAt.After(time.Now()) {
				return workflowDomain.ErrReservationUnavailable
			}
		}
		if err := createOrderSnapshot(tx, order); err != nil {
			return err
		}
		if attempt != nil {
			if order.Contact == nil {
				return fmt.Errorf("order contact is required")
			}
			if err := tx.Create(&contactDetailsRecord{OrderID: order.ID, Email: order.Contact.Email, Locale: order.Contact.Locale}).Error; err != nil {
				return err
			}
		}
		if r.syncEnabled {
			if err := enqueueOrderCreated(tx, order); err != nil {
				return err
			}
		}
		result := tx.Model(&reservationRecord{}).Where("id IN ? AND order_id IS NULL AND status = ?", reservationIDs, "active").Updates(map[string]any{"order_id": order.ID, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(reservationIDs)) {
			return workflowDomain.ErrReservationUnavailable
		}
		if attempt != nil {
			if attempt.ExpiresAt.IsZero() {
				attempt.ExpiresAt = order.ExpiresAt
			}
			status := "creating"
			if markPaid {
				status = "created"
			}
			if err := tx.Create(&paymentCheckoutAttemptRecord{OrderID: order.ID, Provider: attempt.Provider, IdempotencyKey: attempt.IdempotencyKey, Amount: attempt.Amount.Amount, Currency: attempt.Amount.Currency, Status: status, Attempts: 1, ExpiresAt: attempt.ExpiresAt}).Error; err != nil {
				return err
			}
		}
		for _, hook := range r.hooks {
			if err := hook.BeforeCreatePending(transaction.WithContext(ctx, tx), order); err != nil {
				return err
			}
		}
		if r.events != nil {
			event, err := eventsDomain.NewCheckoutStartedEvent(order.ID, order.CartID, time.Now().UTC())
			if err != nil {
				return err
			}
			if err := r.events.Publish(transaction.WithContext(ctx, tx), event); err != nil {
				return err
			}
		}
		if markPaid {
			var state orderStateRecord
			if err := tx.First(&state, "id = ?", order.ID).Error; err != nil {
				return err
			}
			return r.completePending(ctx, tx, state, order.ID, ordersDomain.StatusPaid, nil)
		}
		return nil
	})
}

// orderCreatedEvent is intentionally a narrow, versioned transport contract.
// The ERP adapter may load its own projection later, but this durable event is
// enough to request an idempotent export without persisting customer PII,
// browser payment credentials or localized catalog text in Sync.
type orderCreatedEvent struct {
	Version  int                `json:"version"`
	OrderID  uuid.UUID          `json:"order_id"`
	Number   string             `json:"number"`
	Currency string             `json:"currency"`
	Subtotal int64              `json:"subtotal_amount"`
	Tax      int64              `json:"tax_amount"`
	Shipping int64              `json:"shipping_amount"`
	Total    int64              `json:"total_amount"`
	Items    []orderCreatedItem `json:"items"`
}

type orderCreatedItem struct {
	VariantID *uuid.UUID `json:"variant_id,omitempty"`
	SKU       string     `json:"sku,omitempty"`
	Quantity  int        `json:"quantity"`
}

func enqueueOrderCreated(tx *gorm.DB, order *ordersDomain.Order) error {
	items := make([]orderCreatedItem, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, orderCreatedItem{VariantID: item.VariantID, SKU: item.SKU, Quantity: item.Quantity})
	}
	payload, err := json.Marshal(orderCreatedEvent{
		Version: 1, OrderID: order.ID, Number: order.Number, Currency: order.Total.Currency,
		Subtotal: order.Subtotal.Amount, Tax: order.Tax.Amount, Shipping: order.Shipping.Amount,
		Total: order.Total.Amount, Items: items,
	})
	if err != nil {
		return fmt.Errorf("marshal sync order.created event: %w", err)
	}
	return tx.Create(&syncOutboxRecord{
		ID: uuid.New(), Topic: "order.created", AggregateID: order.ID, IdempotencyKey: order.ID,
		Payload: string(payload), Status: "pending",
	}).Error
}

func (r *Repository) CancelPending(ctx context.Context, orderID uuid.UUID) error {
	return r.transition(ctx, orderID, nil, ordersDomain.StatusCancelled)
}

func (r *Repository) RegisterPayment(ctx context.Context, attempt workflowDomain.PaymentAttempt) error {
	return r.withTransaction(ctx, func(tx *gorm.DB) error {
		var order orderStateRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ?", attempt.OrderID).Error; err != nil {
			return workflowDomain.ErrPaymentMismatch
		}
		if order.Status != ordersDomain.StatusPendingPayment || order.PaymentProvider != attempt.Provider || order.TotalAmount != attempt.Amount.Amount || order.Currency != attempt.Amount.Currency {
			return workflowDomain.ErrPaymentMismatch
		}
		var existing paymentRecord
		err := tx.First(&existing, "order_id = ? AND provider = ?", attempt.OrderID, attempt.Provider).Error
		if err == nil {
			if existing.ProviderReference == attempt.ProviderReference && existing.Amount == attempt.Amount.Amount && existing.Currency == attempt.Amount.Currency {
				return nil
			}
			return workflowDomain.ErrPaymentMismatch
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := tx.Create(&paymentRecord{ID: uuid.New(), OrderID: attempt.OrderID, Provider: attempt.Provider, ProviderReference: attempt.ProviderReference, Status: "pending", Amount: attempt.Amount.Amount, Currency: attempt.Amount.Currency}).Error; err != nil {
			return err
		}
		result := tx.Model(&paymentCheckoutAttemptRecord{}).Where("order_id = ? AND provider = ? AND status IN ?", attempt.OrderID, attempt.Provider, []string{"creating", "processing"}).Updates(map[string]any{"status": "created", "provider_reference": attempt.ProviderReference, "locked_at": nil, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return workflowDomain.ErrPaymentMismatch
		}
		return nil
	})
}

func (r *Repository) RecordCheckoutAttempt(ctx context.Context, req workflowDomain.CheckoutAttemptRequest) error {
	return r.withTransaction(ctx, func(tx *gorm.DB) error {
		var existing paymentCheckoutAttemptRecord
		err := tx.First(&existing, "order_id = ?", req.OrderID).Error
		if err == nil {
			if existing.IdempotencyKey != req.IdempotencyKey || existing.Provider != req.Provider {
				return errors.New("idempotency key or provider mismatch for existing checkout attempt")
			}
			return tx.Model(&existing).Updates(map[string]any{
				"attempts":   existing.Attempts + 1,
				"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
			}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return tx.Create(&paymentCheckoutAttemptRecord{
			OrderID:        req.OrderID,
			Provider:       req.Provider,
			IdempotencyKey: req.IdempotencyKey,
			Amount:         req.Amount.Amount,
			Currency:       req.Amount.Currency,
			Status:         "creating",
			Attempts:       1,
			ExpiresAt:      time.Now().UTC().Add(30 * time.Minute),
		}).Error
	})
}

func (r *Repository) FindCheckoutAttempt(ctx context.Context, idempotencyKey string) (*workflowDomain.CheckoutAttempt, error) {
	var record paymentCheckoutAttemptRecord
	if err := r.db.WithContext(ctx).Where("idempotency_key = ?", idempotencyKey).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var order orderStateRecord
	if err := r.db.WithContext(ctx).First(&order, "id = ?", record.OrderID).Error; err != nil {
		return nil, err
	}
	amount, err := money.New(record.Amount, record.Currency)
	if err != nil {
		return nil, err
	}
	return &workflowDomain.CheckoutAttempt{OrderID: record.OrderID, OrderNumber: order.Number, OrderStatus: order.Status, Provider: record.Provider, IdempotencyKey: record.IdempotencyKey, Amount: amount, Status: record.Status, Attempts: record.Attempts, CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt}, nil
}

func (r *Repository) ClaimPendingCheckoutAttempt(ctx context.Context, olderThan, lease time.Duration) (*workflowDomain.CheckoutAttempt, error) {
	now := time.Now().UTC()
	var claimed *workflowDomain.CheckoutAttempt
	err := r.withTransaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Model(&paymentCheckoutAttemptRecord{}).
			Where("status = ? AND locked_at < ?", "processing", now.Add(-lease)).
			Updates(map[string]any{"status": "creating", "locked_at": nil, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
			return err
		}
		var record paymentCheckoutAttemptRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND updated_at < ?", "creating", now.Add(-olderThan)).Order("updated_at").First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if err := tx.Model(&record).Updates(map[string]any{"status": "processing", "locked_at": now, "attempts": gorm.Expr("attempts + 1"), "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
			return err
		}
		amount, err := money.New(record.Amount, record.Currency)
		if err != nil {
			return err
		}
		claimed = &workflowDomain.CheckoutAttempt{OrderID: record.OrderID, Provider: record.Provider, IdempotencyKey: record.IdempotencyKey, Amount: amount, Status: "processing", Attempts: record.Attempts + 1, CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt}
		return nil
	})
	return claimed, err
}

func (r *Repository) MarkCheckoutAttemptFailed(ctx context.Context, orderID uuid.UUID) error {
	result := r.db.WithContext(ctx).Model(&paymentCheckoutAttemptRecord{}).
		Where("order_id = ? AND status IN ?", orderID, []string{"creating", "processing"}).
		Updates(map[string]any{
			"status":     "failed",
			"locked_at":  nil,
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return workflowDomain.ErrPaymentMismatch
	}
	return nil
}

func (r *Repository) RetryCheckoutAttempt(ctx context.Context, orderID uuid.UUID, cause error) error {
	return r.db.WithContext(ctx).Model(&paymentCheckoutAttemptRecord{}).
		Where("order_id = ? AND status = ?", orderID, "processing").
		Updates(map[string]any{"status": "creating", "locked_at": nil, "last_error": fmt.Sprint(cause), "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error
}

func (r *Repository) MarkPaid(ctx context.Context, confirmation workflowDomain.PaymentConfirmation) error {
	return r.transition(ctx, confirmation.OrderID, &confirmation, ordersDomain.StatusPaid)
}

func (r *Repository) MarkFailed(ctx context.Context, confirmation workflowDomain.PaymentConfirmation) error {
	return r.transition(ctx, confirmation.OrderID, &confirmation, ordersDomain.StatusCancelled)
}

func (r *Repository) MarkRefunded(ctx context.Context, confirmation workflowDomain.PaymentConfirmation) error {
	return r.withTransaction(ctx, func(tx *gorm.DB) error {
		var order orderStateRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ?", confirmation.OrderID).Error; err != nil {
			return workflowDomain.ErrInvalidOrderTransition
		}
		var payment paymentRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&payment, "order_id = ? AND provider = ?", confirmation.OrderID, confirmation.Provider).Error; err != nil {
			return workflowDomain.ErrPaymentMismatch
		}
		if order.PaymentProvider != confirmation.Provider || order.TotalAmount != confirmation.Amount.Amount || order.Currency != confirmation.Amount.Currency || payment.ProviderReference != confirmation.ProviderReference || payment.Amount != confirmation.Amount.Amount || payment.Currency != confirmation.Amount.Currency {
			return workflowDomain.ErrPaymentMismatch
		}
		if order.Status == ordersDomain.StatusRefunded && payment.Status == "refunded" {
			return nil
		}
		if order.Status != ordersDomain.StatusPaid || payment.Status != "paid" {
			return workflowDomain.ErrInvalidOrderTransition
		}
		if err := tx.Model(&order).Update("status", ordersDomain.StatusRefunded).Error; err != nil {
			return err
		}
		if err := tx.Model(&payment).Update("status", "refunded").Error; err != nil {
			return err
		}
		for _, hook := range r.hooks {
			if err := hook.BeforeOrderTransition(transaction.WithContext(ctx, tx), confirmation.OrderID, ordersDomain.StatusRefunded); err != nil {
				return err
			}
		}
		if r.events != nil {
			event, err := eventsDomain.NewOrderRefundedEvent(confirmation.OrderID, confirmation.Amount, time.Now().UTC())
			if err != nil {
				return err
			}
			if err := r.events.Publish(transaction.WithContext(ctx, tx), event); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) transition(ctx context.Context, orderID uuid.UUID, confirmation *workflowDomain.PaymentConfirmation, targetStatus string) error {
	return r.withTransaction(ctx, func(tx *gorm.DB) error {
		var order orderStateRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ?", orderID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return workflowDomain.ErrInvalidOrderTransition
			}
			return err
		}
		var payment paymentRecord
		if confirmation != nil {
			// A verified paid callback may arrive after the checkout was expired
			// while the gateway checkout reference could not be persisted. In that
			// case there is deliberately no payments row to validate against. The
			// immutable order snapshot is sufficient to acknowledge the callback
			// and durably open a manual-reconciliation case; never retry a captured
			// payment indefinitely.
			if order.Status == ordersDomain.StatusCancelled && targetStatus == ordersDomain.StatusPaid {
				if order.PaymentProvider != confirmation.Provider || order.TotalAmount != confirmation.Amount.Amount || order.Currency != confirmation.Amount.Currency {
					return workflowDomain.ErrPaymentMismatch
				}
				return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "order_id"}, {Name: "provider"}, {Name: "provider_reference"}, {Name: "reason"}}, DoNothing: true}).Create(&paymentAnomalyRecord{ID: uuid.New(), OrderID: orderID, Provider: confirmation.Provider, ProviderReference: confirmation.ProviderReference, Amount: confirmation.Amount.Amount, Currency: confirmation.Amount.Currency, Reason: "paid_after_cancelled", Status: "open"}).Error
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&payment, "order_id = ? AND provider = ?", orderID, confirmation.Provider).Error; err != nil {
				return workflowDomain.ErrPaymentMismatch
			}
			if order.PaymentProvider != confirmation.Provider || payment.ProviderReference != confirmation.ProviderReference || payment.Amount != confirmation.Amount.Amount || payment.Currency != confirmation.Amount.Currency {
				return workflowDomain.ErrPaymentMismatch
			}
			if order.Status == targetStatus && ((targetStatus == ordersDomain.StatusPaid && payment.Status == "paid") || (targetStatus == ordersDomain.StatusCancelled && (payment.Status == "failed" || payment.Status == "cancelled"))) {
				return nil
			}
		}
		if order.Status != ordersDomain.StatusPendingPayment {
			return workflowDomain.ErrInvalidOrderTransition
		}
		var paymentToUpdate *paymentRecord
		if confirmation != nil {
			paymentToUpdate = &payment
		}
		return r.completePending(ctx, tx, order, orderID, targetStatus, paymentToUpdate)
	})
}

// withTransaction joins an outer Admin/Workflow transaction when one is
// carried in context. Starting a new transaction here would commit a status
// change even if its companion Outbox append later fails.
func (r *Repository) withTransaction(ctx context.Context, fn func(*gorm.DB) error) error {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return fn(tx.WithContext(ctx))
	}
	return r.db.WithContext(ctx).Transaction(fn)
}

func (r *Repository) completePending(ctx context.Context, tx *gorm.DB, order orderStateRecord, orderID uuid.UUID, targetStatus string, payment *paymentRecord) error {
	var reservations []reservationRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ?", orderID).Find(&reservations).Error; err != nil {
		return err
	}
	if len(reservations) == 0 {
		return workflowDomain.ErrReservationUnavailable
	}
	for _, reservation := range reservations {
		if reservation.Status != "active" {
			return workflowDomain.ErrReservationUnavailable
		}
		var result *gorm.DB
		if targetStatus == ordersDomain.StatusPaid {
			result = tx.Exec(`UPDATE stock_items SET quantity_on_hand = quantity_on_hand - ?, quantity_reserved = quantity_reserved - ?, updated_at = CURRENT_TIMESTAMP WHERE variant_id = ? AND warehouse_id = ? AND quantity_reserved >= ?`, reservation.Quantity, reservation.Quantity, reservation.VariantID, reservation.WarehouseID, reservation.Quantity)
		} else {
			result = tx.Exec(`UPDATE stock_items SET quantity_reserved = quantity_reserved - ?, updated_at = CURRENT_TIMESTAMP WHERE variant_id = ? AND warehouse_id = ? AND quantity_reserved >= ?`, reservation.Quantity, reservation.VariantID, reservation.WarehouseID, reservation.Quantity)
		}
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return workflowDomain.ErrReservationUnavailable
		}
	}
	reservationStatus := "released"
	if targetStatus == ordersDomain.StatusPaid {
		reservationStatus = "committed"
	}
	if err := tx.Model(&reservationRecord{}).Where("order_id = ? AND status = ?", orderID, "active").Updates(map[string]any{"status": reservationStatus, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
		return err
	}
	if err := tx.Model(&order).Update("status", targetStatus).Error; err != nil {
		return err
	}
	if payment != nil {
		paymentStatus := "failed"
		if targetStatus == ordersDomain.StatusPaid {
			paymentStatus = "paid"
		}
		if err := tx.Model(payment).Update("status", paymentStatus).Error; err != nil {
			return err
		}
	}
	if targetStatus == ordersDomain.StatusPaid && order.DeliveryProvider != "" {
		var details deliveryDetailsRecord
		if err := tx.First(&details, "order_id = ?", orderID).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "order_id"}}, DoNothing: true}).Create(&deliveryJobRecord{ID: uuid.New(), OrderID: orderID, Provider: order.DeliveryProvider, IdempotencyKey: uuid.NewSHA1(orderID, []byte("delivery:create")), Status: "pending"}).Error; err != nil {
			return err
		}
	}
	if targetStatus == ordersDomain.StatusPaid && order.CartID != nil {
		if err := tx.Table("carts").Where("id = ? AND status = ?", *order.CartID, "active").Updates(map[string]any{"status": "converted", "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
			return err
		}
	}
	for _, hook := range r.hooks {
		if err := hook.BeforeOrderTransition(transaction.WithContext(ctx, tx), orderID, targetStatus); err != nil {
			return err
		}
	}
	if targetStatus == ordersDomain.StatusPaid && r.events != nil {
		event, err := eventsDomain.NewOrderPaidEvent(orderID, order.Number, money.Money{Amount: order.TotalAmount, Currency: order.Currency}, time.Now().UTC())
		if err != nil {
			return err
		}
		if err := r.events.Publish(transaction.WithContext(ctx, tx), event); err != nil {
			return err
		}
	}
	if targetStatus == ordersDomain.StatusCancelled {
		if err := tx.Model(&paymentCheckoutAttemptRecord{}).Where("order_id = ? AND status IN ?", orderID, []string{"creating", "processing", "created"}).Updates(map[string]any{"status": "failed", "locked_at": nil, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ExpirePendingCheckout claims one expired pending order with SKIP LOCKED and
// releases every local reservation through the same workflow transaction.
func (r *Repository) ExpirePendingCheckout(ctx context.Context, now time.Time) (bool, error) {
	expired := false
	err := r.withTransaction(ctx, func(tx *gorm.DB) error {
		var order orderStateRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status = ? AND expires_at <= ?", ordersDomain.StatusPendingPayment, now).Order("expires_at").First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if err := r.completePending(ctx, tx, order, order.ID, ordersDomain.StatusCancelled, nil); err != nil {
			return err
		}
		expired = true
		return nil
	})
	return expired, err
}

func createOrderSnapshot(tx *gorm.DB, order *ordersDomain.Order) error {
	var cartID *uuid.UUID
	if order.CartID != uuid.Nil {
		cartID = &order.CartID
	}
	record := orderRecord{ID: order.ID, CartID: cartID, Number: order.Number, CustomerID: order.CustomerID, Status: order.Status, Currency: order.Total.Currency, SubtotalAmount: order.Subtotal.Amount, TaxAmount: order.Tax.Amount, ShippingAmount: order.Shipping.Amount, TotalAmount: order.Total.Amount, PaymentProvider: order.PaymentProvider, DeliveryProvider: order.DeliveryProvider, ExpiresAt: order.ExpiresAt}
	if order.Promotion != nil {
		code := order.Promotion.Code
		record.AppliedPromoCode = &code
		record.DiscountAmount = order.Promotion.Discount.Amount
		record.PromoSnapshotType = &order.Promotion.Type
		record.PromoSnapshotValue = &order.Promotion.Value
		if order.Promotion.Currency != "" {
			currency := order.Promotion.Currency
			record.PromoSnapshotCurrency = &currency
		}
	}
	if err := tx.Create(&record).Error; err != nil {
		return err
	}
	if order.Delivery != nil {
		details := order.Delivery
		if err := tx.Create(&deliveryDetailsRecord{OrderID: order.ID, RecipientName: details.RecipientName, RecipientPhone: details.RecipientPhone, CountryCode: details.CountryCode, PostalCode: details.PostalCode, City: details.City, Line1: details.Line1, Line2: details.Line2, LocalityID: details.LocalityID, ServicePointID: details.ServicePointID}).Error; err != nil {
			return err
		}
	}
	items := make([]itemRecord, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, itemRecord{ID: uuid.New(), OrderID: order.ID, VariantID: item.VariantID, ProductName: item.ProductName, SKU: item.SKU, Quantity: item.Quantity, UnitPriceAmount: item.UnitPrice.Amount, TotalAmount: item.Total.Amount, Currency: item.Total.Currency, UnitWeightGrams: item.UnitWeightGrams})
	}
	return tx.Create(&items).Error
}

func lockReservations(tx *gorm.DB, reservationIDs []uuid.UUID) ([]reservationRecord, error) {
	var reservations []reservationRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", reservationIDs).Find(&reservations).Error; err != nil {
		return nil, err
	}
	if len(reservations) != len(reservationIDs) {
		return nil, workflowDomain.ErrReservationUnavailable
	}
	return reservations, nil
}

var _ workflowDomain.Repository = (*Repository)(nil)
