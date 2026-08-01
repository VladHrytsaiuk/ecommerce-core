package http

import "github.com/google/uuid"

// ==========================================
// Request DTOs
// ==========================================

// CreateOrderRequest запит на створення замовлення
type CreateOrderRequest struct {
	Delivery     DeliveryRequest  `json:"delivery" binding:"required"`
	Customer     CustomerRequest  `json:"customer" binding:"required"`
	AdminComment string           `json:"admin_comment"`
	PayTypes     string           `json:"paytypes"`
}

// DeliveryRequest інформація про доставку
type DeliveryRequest struct {
	Provider      string `json:"provider" binding:"required"`
	DeliveryType  string `json:"delivery_type" binding:"required"`
	CityRef       string `json:"city_ref" binding:"required"`
	CityName      string `json:"city_name" binding:"required"`
	WarehouseRef  string `json:"warehouse_ref" binding:"required"`
	WarehouseName string `json:"warehouse_name" binding:"required"`
}

// CustomerRequest дані покупця (для замовлення)
type CustomerRequest struct {
	Email     string `json:"email" binding:"required,email"`
	FirstName string `json:"first_name" binding:"required,min=1"`
	LastName  string `json:"last_name" binding:"required,min=1"`
	Phone     string `json:"phone" binding:"required"`
}

// ==========================================
// Response DTOs
// ==========================================

// CreateOrderResponse відповідь після створення замовлення
type CreateOrderResponse struct {
	OrderID     uuid.UUID `json:"order_id"`
	OrderNumber int64     `json:"order_number"`
	TotalPrice  int       `json:"total_price"`
	Status      string    `json:"status"`
	PaymentURL  string    `json:"payment_url"`
	IsNewUser   bool      `json:"is_new_user"`
	SetupToken  *string   `json:"setup_token,omitempty"`
}

// OrderBriefResponse мінімалістична відповідь для списків
type OrderBriefResponse struct {
	ID          uuid.UUID      `json:"id"`
	OrderNumber int64          `json:"order_number"`
	CreatedAt   string         `json:"created_at"`
	Status      OrderStatusDTO `json:"status"`
	TotalPrice  int            `json:"total_price"`
	PaymentURL  string         `json:"payment_url,omitempty"`
}

// OrderResponse детальна відповідь замовлення
type OrderResponse struct {
	ID           uuid.UUID         `json:"id"`
	OrderNumber  int64             `json:"order_number"`
	Status       OrderStatusDTO    `json:"status"`
	FirstName      string            `json:"first_name"`
	LastName       string            `json:"last_name"`
	Email          string            `json:"email"`
	Phone          string            `json:"phone"`
	TotalPrice     int               `json:"total_price"`
	PromoCode      *string           `json:"promo_code,omitempty"`
	DiscountAmount int               `json:"discount_amount"`
	Items          []OrderItemDTO    `json:"items"`
	Delivery       *DeliveryDTO      `json:"delivery"`
	AdminComment   string            `json:"admin_comment,omitempty"`
	CreatedAt      string            `json:"created_at"`
	PaymentURL     string            `json:"payment_url,omitempty"`
}

// OrderStatusDTO статус замовлення для API
type OrderStatusDTO struct {
	ID   int    `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// OrderItemDTO елемент замовлення для API
type OrderItemDTO struct {
	ID          uuid.UUID `json:"id"`
	VariationID uuid.UUID `json:"variation_id"`
	ProductName string    `json:"product_name,omitempty"`
	Slug        string    `json:"slug,omitempty"`
	ImageURL    string    `json:"image_url,omitempty"`
	Price           int       `json:"price"`
	Quantity        int       `json:"quantity"`
	TotalPrice      int       `json:"total_price"`
	DiscountAmount  int       `json:"discount_amount"`
	FinalTotalPrice int       `json:"final_total_price"`
}

// DeliveryDTO доставка для API
type DeliveryDTO struct {
	Provider       string `json:"provider"`
	DeliveryType   string `json:"delivery_type"`
	CityName       string `json:"city_name"`
	WarehouseName  string `json:"warehouse_name"`
	TrackingNumber string `json:"tracking_number,omitempty"`
	Status         string `json:"status,omitempty"`
}

// ErrorResponse стандартна помилка
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// OrderPaymentStatusResponse відповідь зі статусом оплати замовлення
type OrderPaymentStatusResponse struct {
	Status string `json:"status" example:"paid"` // Статус оплати (pending, paid, failed)
}
