//go:build legacy && ignore
// +build legacy,ignore

package http

import "github.com/google/uuid"

// ManagerOrderResponse детальна відповідь замовлення для менеджера
type ManagerOrderResponse struct {
	ID           uuid.UUID        `json:"id"`
	OrderNumber  int64            `json:"order_number"`
	Status       OrderStatusDTO   `json:"status"`
	FirstName    string           `json:"first_name"`
	LastName     string           `json:"last_name"`
	Email        string           `json:"email"`
	Phone        string           `json:"phone"`
	TotalPrice   int              `json:"total_price"`
	TTNNumber    string           `json:"ttn_number,omitempty"`
	TTNCreatedAt string           `json:"ttn_created_at,omitempty"`
	Items        []ManagerItemDTO `json:"items"`
	Delivery     *DeliveryDTO     `json:"delivery"`
	CreatedAt    string           `json:"created_at"`
}

// ManagerItemDTO елемент замовлення для менеджера
type ManagerItemDTO struct {
	ID          uuid.UUID `json:"id"`
	VariationID uuid.UUID `json:"variation_id"`
	ProductName string    `json:"product_name,omitempty"`
	ImageURL    string    `json:"image_url,omitempty"`
	Price       int       `json:"price"`
	Quantity    int       `json:"quantity"`
	TotalPrice  int       `json:"total_price"`
}

// ConfirmOrderResponse відповідь після підтвердження замовлення та створення ТТН
type ConfirmOrderResponse struct {
	OrderNumber int64  `json:"order_number"`
	TTNNumber   string `json:"ttn_number"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}
