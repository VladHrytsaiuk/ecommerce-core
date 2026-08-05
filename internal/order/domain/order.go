//go:build legacy && ignore
// +build legacy,ignore

package domain

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"github.com/google/uuid"
)

// ==========================================
// Domain Errors
// ==========================================

var (
	ErrOrderNotFound            = errors.New("order not found")
	ErrEmptyCart                = errors.New("cart is empty")
	ErrInactiveItem             = errors.New("one or more items are no longer available")
	ErrInvalidStatus            = errors.New("invalid order status transition")
	ErrNoGuestData              = errors.New("guest checkout requires email, first_name, last_name, phone")
	ErrNoDeliveryData           = errors.New("delivery information is required")
	ErrNoIdentifier             = errors.New("either user_id or guest data must be provided")
	ErrInvalidManagerToken      = errors.New("invalid or expired manager token")
	ErrTTNAlreadyCreated        = errors.New("TTN already created for this order")
	ErrOrderNotPaid             = errors.New("order must be in paid status to create TTN")
	ErrOrderNotProcessing       = errors.New("order must be in processing status")
	ErrOrderAlreadyCancelled    = errors.New("order is already cancelled")
	ErrMinOrderAmountNotReached = errors.New("minimum order amount not reached")
	ErrCancelBlockedAfterTTN    = errors.New("cannot cancel order after TTN is created")
)

// ==========================================
// Order Status Constants
// ==========================================

const (
	StatusPendingPayment = 1
	StatusPaid           = 2
	StatusProcessing     = 3
	StatusShipped        = 4
	StatusDelivered      = 5
	StatusCancelled      = 6
	StatusRefunded       = 7
)

// ==========================================
// Status History Source Constants
// ==========================================

const (
	SourcePaymentWebhook = "payment_webhook"
	SourceManagerLink    = "manager_link"
	SourceAdmin          = "admin"
	SourceNovaPoshta     = "nova_poshta"
	SourceSystem         = "system"
	SourceUser           = "user"
)

// ==========================================
// Entities
// ==========================================

// LocalizedMap допоміжний тип для роботи з JSONB перекладами (реюз з product domain)
type LocalizedMap map[string]string

func (m LocalizedMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (m *LocalizedMap) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to scan LocalizedMap: expected []byte or string, got %T", value)
	}

	result := make(map[string]string)
	if err := json.Unmarshal(bytes, &result); err != nil {
		return err
	}
	*m = LocalizedMap(result)
	return nil
}

// OrderStatus довідникова таблиця статусів замовлення
type OrderStatus struct {
	ID        int          `gorm:"primaryKey" json:"id"`
	Code      string       `gorm:"type:varchar(50);not null" json:"code"`
	Name      LocalizedMap `gorm:"type:jsonb;not null" json:"name"`
	SortOrder int          `gorm:"not null;default:0" json:"sort_order"`
}

func (OrderStatus) TableName() string {
	return "order_status"
}

// Order замовлення
type Order struct {
	ID             uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderNumber    int64      `gorm:"not null;default:(-)" json:"order_number"`
	UserID         *uuid.UUID `gorm:"type:uuid" json:"user_id"`
	StatusID       int        `gorm:"not null" json:"status_id"`
	FirstName      string     `gorm:"type:varchar(100)" json:"first_name"`
	LastName       string     `gorm:"type:varchar(100)" json:"last_name"`
	Email          string     `gorm:"type:varchar(255)" json:"email"`
	Phone          string     `gorm:"type:varchar(20)" json:"phone"`
	TotalPrice     int        `gorm:"not null;default:0" json:"total_price"` // with discount
	PromoCode      *string    `gorm:"type:varchar(50)" json:"promo_code,omitempty"`
	DiscountAmount int        `gorm:"not null;default:0" json:"discount_amount"`
	AdminComment   string     `gorm:"type:text" json:"admin_comment"`
	PayTypes       string     `gorm:"column:paytypes;type:varchar(255)" json:"paytypes,omitempty"`
	CreatedAt      time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Manager token (hashed)
	ManagerTokenHash      string     `gorm:"type:varchar(64)" json:"-"`
	ManagerTokenExpiresAt *time.Time `gorm:"type:timestamptz" json:"-"`

	PaymentReminderSentAt *time.Time `gorm:"type:timestamptz" json:"payment_reminder_sent_at,omitempty"`

	// TTN (товарно-транспортна накладна)
	TTNNumber          string     `gorm:"column:ttn_number;type:varchar(100)" json:"ttn_number,omitempty"`
	TTNRef             string     `gorm:"column:ttn_ref;type:varchar(100)" json:"-"`
	TTNCreatedAt       *time.Time `gorm:"column:ttn_created_at;type:timestamptz" json:"ttn_created_at,omitempty"`
	CarrierStatus      string     `gorm:"type:varchar(50)" json:"carrier_status,omitempty"`
	CarrierRawResponse *string    `gorm:"type:jsonb" json:"-"`

	// Relationships
	Status        OrderStatus          `gorm:"foreignKey:StatusID" json:"status,omitempty"`
	Items         []OrderItem          `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE;" json:"items,omitempty"`
	Delivery      *Delivery            `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE;" json:"delivery,omitempty"`
	StatusHistory []OrderStatusHistory `gorm:"foreignKey:OrderID" json:"status_history,omitempty"`
}

func (Order) TableName() string {
	return `"order"`
}

// OrderItem елемент замовлення (snapshot цін на момент покупки)
type OrderItem struct {
	ID              uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderID         uuid.UUID `gorm:"type:uuid;not null" json:"order_id"`
	VariationID     uuid.UUID `gorm:"type:uuid;not null" json:"variation_id"`
	Price           int       `gorm:"not null;default:0" json:"price"` // ціна за одиницю на момент покупки (копійки)
	Quantity        int       `gorm:"not null;default:1" json:"quantity"`
	TotalPrice      int       `gorm:"not null;default:0" json:"total_price"` // price * quantity
	DiscountAmount  int       `gorm:"not null;default:0" json:"discount_amount"`
	FinalTotalPrice int       `gorm:"not null;default:0" json:"final_total_price"` // TotalPrice - DiscountAmount
	CreatedAt       time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	// Relationships
	Variation productDomain.ProductVariation `gorm:"foreignKey:VariationID" json:"variation,omitempty"`
}

func (OrderItem) TableName() string {
	return "order_item"
}

// Delivery інформація про доставку замовлення
type Delivery struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderID        uuid.UUID `gorm:"type:uuid;not null" json:"order_id"`
	Provider       string    `gorm:"type:varchar(50)" json:"provider"`
	DeliveryType   string    `gorm:"type:varchar(50)" json:"delivery_type"`
	CityRef        string    `gorm:"type:varchar(100)" json:"city_ref"`
	CityName       string    `gorm:"type:varchar(100)" json:"city_name"`
	WarehouseRef   string    `gorm:"type:varchar(100)" json:"warehouse_ref"`
	WarehouseName  string    `gorm:"type:varchar(255)" json:"warehouse_name"`
	TrackingNumber string    `gorm:"type:varchar(100)" json:"tracking_number"`
	Status         string    `gorm:"type:varchar(50)" json:"status"`
	IsFreeDelivery bool      `gorm:"not null;default:false" json:"is_free_delivery"`
	CreatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Delivery) TableName() string {
	return "delivery"
}

// OrderStatusHistory запис історії зміни статусу замовлення
type OrderStatusHistory struct {
	ID           uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderID      uuid.UUID  `gorm:"type:uuid;not null" json:"order_id"`
	FromStatusID *int       `gorm:"" json:"from_status_id"`
	ToStatusID   int        `gorm:"not null" json:"to_status_id"`
	Source       string     `gorm:"type:varchar(50);not null" json:"source"`
	AdminUserID  *uuid.UUID `gorm:"type:uuid" json:"admin_user_id,omitempty"`
	Comment      *string    `gorm:"type:text" json:"comment,omitempty"`
	CreatedAt    time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	// Relationships for preload
	FromStatus *OrderStatus `gorm:"foreignKey:FromStatusID" json:"from_status,omitempty"`
	ToStatus   OrderStatus  `gorm:"foreignKey:ToStatusID" json:"to_status,omitempty"`
}

func (OrderStatusHistory) TableName() string {
	return "order_status_history"
}

// ==========================================
// Input Types
// ==========================================

// CreateOrderInput вхідні дані для створення замовлення
type CreateOrderInput struct {
	// Customer contact details
	CustomerEmail     string
	CustomerFirstName string
	CustomerLastName  string
	CustomerPhone     string

	// Delivery
	DeliveryProvider      string
	DeliveryType          string
	DeliveryCityRef       string
	DeliveryCityName      string
	DeliveryWarehouseRef  string
	DeliveryWarehouseName string

	AdminComment string
	PayTypes     string
}

// AdminOrderFilters параметри фільтрації та пагінації для адмін-списку замовлень
type AdminOrderFilters struct {
	StatusID *int       `form:"status_id"`
	Search   string     `form:"search"`
	DateFrom *time.Time `form:"date_from" time_format:"2006-01-02"`
	DateTo   *time.Time `form:"date_to" time_format:"2006-01-02"`
	pagination.Params
}

// AdminPaymentInfo — безпечний зріз даних платежу для картки замовлення в адмінці.
// Дані картки та технічні відповіді провайдера сюди не потрапляють.
type AdminPaymentInfo struct {
	Provider      string
	Status        string
	TransactionID string
	Amount        int
	Currency      string
}

// CreateOrderResult результат створення замовлення
type CreateOrderResult struct {
	OrderID     uuid.UUID `json:"order_id"`
	OrderNumber int64     `json:"order_number"`
	TotalPrice  int       `json:"total_price"`
	Status      string    `json:"status"`
	PaymentURL  string    `json:"payment_url,omitempty"`
	IsNewUser   bool      `json:"is_new_user"`
	SetupToken  *string   `json:"setup_token,omitempty"`
}

// ==========================================
// Contracts
// ==========================================

// OrderRepository контракт для роботи з БД замовлень
type OrderRepository interface {
	Create(ctx context.Context, order *Order, items []OrderItem, delivery *Delivery) error
	FindByID(ctx context.Context, id uuid.UUID) (*Order, error)
	FindByOrderNumber(ctx context.Context, orderNumber int64) (*Order, error)
	GetOrderStatusByID(ctx context.Context, orderID uuid.UUID) (int, error)
	GetForUpdate(ctx context.Context, orderNumber int64) (*Order, error)
	FindByIDForUpdate(ctx context.Context, orderID uuid.UUID) (*Order, error)
	WithTransaction(ctx context.Context, fn func(ctx context.Context, txRepo OrderRepository) error) error
	FindByUserID(ctx context.Context, userID uuid.UUID, pgn pagination.Params) ([]Order, int64, error)
	FindAllOrders(ctx context.Context, filters AdminOrderFilters) ([]Order, int64, error)
	UpdateStatus(ctx context.Context, orderID uuid.UUID, statusID int) error
	Update(ctx context.Context, order *Order) error
	SetManagerToken(ctx context.Context, orderID uuid.UUID, hash string, expiresAt time.Time) error
	SetTTNData(ctx context.Context, orderID uuid.UUID, ttnNumber, ttnRef, carrierStatus string, rawResponse *string) error
	UpdateAdminComment(ctx context.Context, orderID uuid.UUID, comment string) error
	UpdateCarrierData(ctx context.Context, orderID uuid.UUID, carrierStatus string, rawResponse *string) error
	GetOrdersForPaymentReminder(ctx context.Context, olderThan time.Time) ([]Order, error)
	GetOrdersForPaymentTimeout(ctx context.Context, olderThan time.Time) ([]Order, error)
	MarkPaymentReminderSent(ctx context.Context, orderID uuid.UUID) error
	GetOrderStatuses(ctx context.Context) ([]OrderStatus, error)
	GetShippedOrders(ctx context.Context) ([]Order, error)
	CreateStatusHistory(ctx context.Context, history *OrderStatusHistory) error
	FindStatusHistory(ctx context.Context, orderID uuid.UUID) ([]OrderStatusHistory, error)
}

// OrderService контракт для бізнес-логіки замовлень
type OrderService interface {
	CreateOrder(ctx context.Context, userID *uuid.UUID, sessionID *string, lang string, input CreateOrderInput) (*CreateOrderResult, error)
	GetByID(ctx context.Context, orderID uuid.UUID) (*Order, error)
	GetOrderStatusByID(ctx context.Context, orderID uuid.UUID) (int, error)
	GetMyOrders(ctx context.Context, userID uuid.UUID, pgn pagination.Params) ([]Order, pagination.Metadata, error)
	CancelOrderByUser(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error
	ProcessPaymentTimeouts(ctx context.Context) error
	GeneratePaymentURL(ctx context.Context, orderID uuid.UUID) (string, error)
}

// ManagerService контракт для менеджерських операцій із замовленнями
type ManagerService interface {
	GetOrderByToken(ctx context.Context, orderNumber int64, plainToken string) (*Order, error)
	ConfirmOrder(ctx context.Context, orderNumber int64, plainToken string) (*Order, error)
	CancelOrder(ctx context.Context, orderNumber int64, plainToken string) error
}

// ConfirmOrderService контракт для спільної логіки підтвердження замовлення
type ConfirmOrderService interface {
	ConfirmOrder(ctx context.Context, orderID uuid.UUID, source string, adminUserID *uuid.UUID) (*Order, error)
}

// AdminOrderService контракт для адмін-операцій із замовленнями
type AdminOrderService interface {
	ListOrders(ctx context.Context, filters AdminOrderFilters) ([]Order, pagination.Metadata, error)
	GetOrderByID(ctx context.Context, orderID uuid.UUID) (*Order, []OrderStatusHistory, *AdminPaymentInfo, error)
	GetOrderStatuses(ctx context.Context) ([]OrderStatus, error)
	UpdateAdminComment(ctx context.Context, orderID uuid.UUID, comment string) error
	ConfirmOrder(ctx context.Context, orderID uuid.UUID, adminUserID uuid.UUID) (*Order, error)
	CancelOrder(ctx context.Context, orderID uuid.UUID, adminUserID uuid.UUID) error
}

// ==========================================
// Manager Token Helpers
// ==========================================

// GenerateManagerAccessToken генерує криптографічно безпечний токен та його SHA-256 хеш.
// Повертає (plainToken, hash, error).
func GenerateManagerAccessToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("failed to generate manager token: %w", err)
	}
	plainToken := hex.EncodeToString(b) // 64 hex chars
	hash := HashManagerToken(plainToken)
	return plainToken, hash, nil
}

// HashManagerToken обчислює SHA-256 хеш від plain token.
func HashManagerToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// ConstantTimeCompareHash порівнює два хеші у constant-time для захисту від timing attacks.
func ConstantTimeCompareHash(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
