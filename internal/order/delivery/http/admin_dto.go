//go:build legacy
// +build legacy

package http

import (
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/google/uuid"
)

// AdminOrderListResponse DTO для списку замовлень в адмінці
type AdminOrderListResponse struct {
	ID                    uuid.UUID      `json:"id"`
	OrderNumber           int64          `json:"order_number"`
	Status                OrderStatusDTO `json:"status"`
	FirstName             string         `json:"first_name"`
	LastName              string         `json:"last_name"`
	Phone                 string         `json:"phone"`
	TotalPrice            int            `json:"total_price"`
	CreatedAt             time.Time      `json:"created_at"`
	TTNNumber             string         `json:"ttn_number,omitempty"`
	DeliveryProvider      string         `json:"delivery_provider,omitempty"`
	DeliveryType          string         `json:"delivery_type,omitempty"`
	DeliveryCityName      string         `json:"delivery_city_name,omitempty"`
	DeliveryWarehouseName string         `json:"delivery_warehouse_name,omitempty"`
	DeliveryStatus        string         `json:"delivery_status,omitempty"`
	CarrierStatus         string         `json:"carrier_status,omitempty"`
	FirstItemName         string         `json:"first_item_name,omitempty"`
	AdditionalItemsCount  int            `json:"additional_items_count"`
}

// AdminOrderDetailsResponse DTO для деталей замовлення
type AdminOrderDetailsResponse struct {
	ID                    uuid.UUID                    `json:"id"`
	OrderNumber           int64                        `json:"order_number"`
	Status                OrderStatusDTO               `json:"status"`
	Customer              AdminCustomerDTO             `json:"customer"`
	Items                 []AdminOrderItemDTO          `json:"items"`
	Delivery              *DeliveryDTO                 `json:"delivery,omitempty"`
	Payment               AdminPaymentInfoDTO          `json:"payment"`
	TTNNumber             string                       `json:"ttn_number,omitempty"`
	TTNCreatedAt          *time.Time                   `json:"ttn_created_at,omitempty"`
	CarrierStatus         string                       `json:"carrier_status,omitempty"`
	PromoCode             *string                      `json:"promo_code,omitempty"`
	DiscountAmount        int                          `json:"discount_amount"`
	History               []AdminOrderStatusHistoryDTO `json:"history"`
	AdminComment          *string                      `json:"admin_comment,omitempty"`
	ManagerTokenExpiresAt *time.Time                   `json:"manager_token_expires_at,omitempty"`
	CreatedAt             time.Time                    `json:"created_at"`
	UpdatedAt             time.Time                    `json:"updated_at"`
}

type AdminCustomerDTO struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
}

type AdminOrderItemDTO struct {
	ID          uuid.UUID `json:"id"`
	VariationID uuid.UUID `json:"variation_id"`
	ProductName string    `json:"product_name"`
	ImageURL    string    `json:"image_url"`
	Price       int       `json:"price"`
	Quantity    int       `json:"quantity"`
	Discount    int       `json:"discount_amount"`
	TotalPrice  int       `json:"total_price"`
}

type AdminPaymentInfoDTO struct {
	TotalPrice    int    `json:"total_price"`
	PayTypes      string `json:"pay_types,omitempty"`
	Provider      string `json:"provider,omitempty"`
	Status        string `json:"status,omitempty"`
	TransactionID string `json:"transaction_id,omitempty"`
	Amount        int    `json:"amount,omitempty"`
	Currency      string `json:"currency,omitempty"`
}

type AdminOrderStatusHistoryDTO struct {
	ID          uuid.UUID       `json:"id"`
	FromStatus  *OrderStatusDTO `json:"from_status,omitempty"`
	ToStatus    OrderStatusDTO  `json:"to_status"`
	Source      string          `json:"source"`
	AdminUserID *uuid.UUID      `json:"admin_user_id,omitempty"`
	Comment     *string         `json:"comment,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// UpdateAdminCommentRequest DTO для оновлення коментаря
type UpdateAdminCommentRequest struct {
	Comment string `json:"comment" binding:"required"`
}

// MapToAdminList маппить масив Order у масив DTO
func MapToAdminList(orders []domain.Order) []AdminOrderListResponse {
	list := make([]AdminOrderListResponse, len(orders))
	for i, o := range orders {
		item := AdminOrderListResponse{
			ID:          o.ID,
			OrderNumber: o.OrderNumber,
			Status: OrderStatusDTO{
				ID:   o.Status.ID,
				Code: o.Status.Code,
				Name: o.Status.Name["uk"], // Default UK
			},
			FirstName:     o.FirstName,
			LastName:      o.LastName,
			Phone:         o.Phone,
			TotalPrice:    o.TotalPrice,
			CreatedAt:     o.CreatedAt,
			TTNNumber:     o.TTNNumber,
			CarrierStatus: o.CarrierStatus,
		}

		if o.Delivery != nil {
			item.DeliveryProvider = o.Delivery.Provider
			item.DeliveryType = o.Delivery.DeliveryType
			item.DeliveryCityName = o.Delivery.CityName
			item.DeliveryWarehouseName = o.Delivery.WarehouseName
			item.DeliveryStatus = o.Delivery.Status
		}

		if len(o.Items) > 0 {
			firstItem := o.Items[0]
			for _, trans := range firstItem.Variation.Product.Translations {
				if trans.LanguageCode == "uk" {
					item.FirstItemName = trans.Name
					break
				}
			}
			item.AdditionalItemsCount = len(o.Items) - 1
		}

		list[i] = item
	}
	return list
}

// MapToAdminDetails маппить Order та History у детальне DTO
func MapToAdminDetails(o *domain.Order, h []domain.OrderStatusHistory, payment *domain.AdminPaymentInfo) AdminOrderDetailsResponse {
	resp := AdminOrderDetailsResponse{
		ID:          o.ID,
		OrderNumber: o.OrderNumber,
		Status: OrderStatusDTO{
			ID:   o.Status.ID,
			Code: o.Status.Code,
			Name: o.Status.Name["uk"],
		},
		Customer: AdminCustomerDTO{
			FirstName: o.FirstName,
			LastName:  o.LastName,
			Email:     o.Email,
			Phone:     o.Phone,
		},
		Payment: AdminPaymentInfoDTO{
			TotalPrice: o.TotalPrice,
			PayTypes:   o.PayTypes,
		},
		ManagerTokenExpiresAt: o.ManagerTokenExpiresAt,
		CreatedAt:             o.CreatedAt,
		UpdatedAt:             o.UpdatedAt,
		TTNNumber:             o.TTNNumber,
		TTNCreatedAt:          o.TTNCreatedAt,
		CarrierStatus:         o.CarrierStatus,
		DiscountAmount:        o.DiscountAmount,
	}
	if payment != nil {
		resp.Payment.Provider = payment.Provider
		resp.Payment.Status = payment.Status
		resp.Payment.TransactionID = payment.TransactionID
		resp.Payment.Amount = payment.Amount
		resp.Payment.Currency = payment.Currency
	}

	if o.PromoCode != nil {
		resp.PromoCode = o.PromoCode
	}

	if o.AdminComment != "" {
		resp.AdminComment = &o.AdminComment
	}

	if o.Delivery != nil {
		resp.Delivery = &DeliveryDTO{
			Provider:       o.Delivery.Provider,
			DeliveryType:   o.Delivery.DeliveryType,
			CityName:       o.Delivery.CityName,
			WarehouseName:  o.Delivery.WarehouseName,
			TrackingNumber: o.Delivery.TrackingNumber,
			Status:         o.Delivery.Status,
		}
	}

	// Items
	items := make([]AdminOrderItemDTO, len(o.Items))
	for i, item := range o.Items {
		dto := AdminOrderItemDTO{
			ID:          item.ID,
			VariationID: item.VariationID,
			Price:       item.Price,
			Quantity:    item.Quantity,
			Discount:    item.DiscountAmount,
			TotalPrice:  item.FinalTotalPrice,
		}
		for _, trans := range item.Variation.Product.Translations {
			if trans.LanguageCode == "uk" {
				dto.ProductName = trans.Name
				break
			}
		}
		for _, img := range item.Variation.Product.Images {
			if img.IsPrimary {
				dto.ImageURL = img.ImageURL
				break
			}
			if dto.ImageURL == "" {
				dto.ImageURL = img.ImageURL
			}
		}
		items[i] = dto
	}
	resp.Items = items

	// History
	hist := make([]AdminOrderStatusHistoryDTO, len(h))
	for i, record := range h {
		dto := AdminOrderStatusHistoryDTO{
			ID:          record.ID,
			Source:      record.Source,
			AdminUserID: record.AdminUserID,
			Comment:     record.Comment,
			CreatedAt:   record.CreatedAt,
			ToStatus: OrderStatusDTO{
				ID:   record.ToStatus.ID,
				Code: record.ToStatus.Code,
				Name: record.ToStatus.Name["uk"],
			},
		}
		if record.FromStatus != nil {
			dto.FromStatus = &OrderStatusDTO{
				ID:   record.FromStatus.ID,
				Code: record.FromStatus.Code,
				Name: record.FromStatus.Name["uk"],
			}
		}
		hist[i] = dto
	}
	resp.History = hist

	return resp
}
