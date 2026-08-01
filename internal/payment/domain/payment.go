package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ==========================================
// Domain Errors
// ==========================================

var (
	ErrPaymentNotFound  = errors.New("payment not found")
	ErrInvalidSignature = errors.New("invalid LiqPay signature")
	ErrAlreadyProcessed = errors.New("payment already processed")
)

// ==========================================
// Entities
// ==========================================

// Payment запис оплати замовлення
type Payment struct {
	ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderID       uuid.UUID `gorm:"type:uuid;not null" json:"order_id"`
	Provider      string    `gorm:"type:varchar(50)" json:"provider"`
	TransactionID string    `gorm:"type:varchar(255)" json:"transaction_id"`
	Amount        int       `gorm:"not null;default:0" json:"amount"`
	Currency      string    `gorm:"type:varchar(10);not null;default:'UAH'" json:"currency"`
	Status        string    `gorm:"type:varchar(50)" json:"status"`
	ErrorMessage  string    `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt     time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Payment) TableName() string {
	return "payment"
}

// LiqPayCallback дані з webhook LiqPay
type LiqPayCallback struct {
	Data      string `form:"data" binding:"required"`
	Signature string `form:"signature" binding:"required"`
}

// LiqPayData декодовані дані з data поля callback
type LiqPayData struct {
	Action       string  `json:"action"`
	Status       string  `json:"status"`
	OrderID      string  `json:"order_id"`
	LiqPayID     int64   `json:"payment_id"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
	Description  string  `json:"description"`
	ErrCode      string  `json:"err_code"`
	ErrDescription string `json:"err_description"`
}

// ==========================================
// Contracts
// ==========================================

// PaymentRepository контракт для роботи з БД платежів
type PaymentRepository interface {
	Create(ctx context.Context, payment *Payment) error
	FindByOrderID(ctx context.Context, orderID uuid.UUID) (*Payment, error)
	UpdateStatus(ctx context.Context, paymentID uuid.UUID, status, transactionID, errorMessage string) error
}

// PaymentService контракт для бізнес-логіки платежів
type PaymentService interface {
	CreatePayment(ctx context.Context, orderID uuid.UUID, amount int) error
	ProcessWebhook(ctx context.Context, data, signature string) error
	GeneratePaymentURL(orderID uuid.UUID, amount int, orderNumber int64, paytypes string) string
	SimulatePayment(ctx context.Context, orderID uuid.UUID) error
	ProcessRefundStub(ctx context.Context, orderID uuid.UUID) error
}
