package http

import (
	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	discountDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
)

const DefaultBundleUnitID = 6

// CartResponse відповідь з повною інформацією про кошик
type CartResponse struct {
	Items              []CartItemResponse `json:"items"`
	TotalCount         int                `json:"total_count"`
	OriginalTotalPrice int                `json:"original_total_price"`
	DiscountAmount     int                `json:"discount_amount"`
	TotalPrice         int                `json:"total_price"`
	PromoCode          *string            `json:"promo_code,omitempty"`
	Shipping           ShippingResponse   `json:"shipping"`
}

// ShippingResponse інформація про безкоштовну доставку
type ShippingResponse struct {
	IsFreeShipping  bool `json:"is_free_shipping"`
	Threshold       int  `json:"threshold"`        // мінімальна сума для безкоштовної доставки (копійки)
	RemainingAmount int  `json:"remaining_amount"` // скільки ще потрібно додати (копійки)
	MinOrderAmount  int  `json:"min_order_amount"` // мінімальна сума замовлення (копійки)
}

// CartItemResponse елемент кошика з інформацією про продукт
type CartItemResponse struct {
	VariationID      uuid.UUID     `json:"variation_id"`
	ProductID        uuid.UUID     `json:"product_id"`
	Slug             string        `json:"slug"`
	SKU              string        `json:"sku"`
	Name             string        `json:"name"`
	BrandName        string        `json:"brand_name"`
	Price            int           `json:"price"`
	OldPrice         *int          `json:"old_price,omitempty"`
	ImageURL         string        `json:"image_url"`
	HoverImageURL    string        `json:"hover_image_url"`
	QuantityValue    float64       `json:"quantity_value"`
	Unit             *UnitResponse `json:"unit,omitempty"`
	Quantity         int           `json:"quantity"`
	TotalPrice       int           `json:"total_price"`
	DiscountAmount   int           `json:"discount_amount"`
	FinalTotalPrice  int           `json:"final_total_price"`
	IsBundle         bool          `json:"is_bundle"`
	IsActive         bool          `json:"is_active"`
}

// UnitResponse спрощена структура для одиниці виміру
type UnitResponse struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
}

// AddToCartRequest тіло запиту для додавання товару до кошика
type AddToCartRequest struct {
	VariationID uuid.UUID `json:"variation_id" binding:"required"`
	Quantity    int       `json:"quantity" binding:"required,min=1"`
}

// UpdateCartItemRequest тіло запиту для оновлення кількості
type UpdateCartItemRequest struct {
	Quantity int `json:"quantity" binding:"required,min=1"`
}

// ApplyPromoRequest тіло запиту для застосування промокоду
type ApplyPromoRequest struct {
	Code string `json:"code" binding:"required"`
}

// SyncCartRequest тіло запиту для синхронізації анонімного кошика
type SyncCartRequest struct {
	SessionID string `json:"session_id"` // опціонально, можемо брати з куки
}

// ErrorResponse стандартна структура для помилок
type ErrorResponse struct {
	Error   string      `json:"error"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// mapToCartResponse конвертує Cart, варіації та shipping у відповідь для фронтенду
func mapToCartResponse(cart *domain.Cart, variations []productDomain.ProductVariation, shipping *domain.ShippingSummary, promoResult *discountDomain.PromoCalculationResult, promoCodeStr *string, lang string) CartResponse {
	shippingResp := ShippingResponse{}
	if shipping != nil {
		shippingResp = ShippingResponse{
			IsFreeShipping:  shipping.IsFreeShipping,
			Threshold:       shipping.Threshold,
			RemainingAmount: shipping.RemainingAmount,
			MinOrderAmount:  shipping.MinOrderAmount,
		}
	}

	if cart == nil || len(cart.Items) == 0 {
		return CartResponse{
			Items:      []CartItemResponse{},
			TotalCount: 0,
			TotalPrice: 0,
			Shipping:   shippingResp,
		}
	}

	// Створюємо map варіацій для швидкого доступу
	variationMap := make(map[uuid.UUID]productDomain.ProductVariation, len(variations))
	for _, v := range variations {
		variationMap[v.ID] = v
	}

	items := make([]CartItemResponse, 0, len(cart.Items))
	totalCount := 0
	totalPrice := 0

	for _, cartItem := range cart.Items {
		v, ok := variationMap[cartItem.VariationID]
		if !ok {
			continue // варіація деактивована або видалена
		}

		itemTotal := v.Price * cartItem.Quantity
		itemDiscount := 0
		if promoResult != nil {
			itemDiscount = promoResult.ItemDiscounts[v.ID]
		}
		itemFinalTotal := itemTotal - itemDiscount

		slug := v.Slug
		if slug == "" {
			if len(v.Product.Translations) > 0 {
				slug = v.Product.Translations[0].Slug
			}
		}
		resp := CartItemResponse{
			VariationID:     v.ID,
			ProductID:       v.ProductID,
			Slug:            slug,
			SKU:             v.SKU,
			Price:           v.Price,
			OldPrice:        v.OldPrice,
			Quantity:        cartItem.Quantity,
			TotalPrice:      itemTotal,
			DiscountAmount:  itemDiscount,
			FinalTotalPrice: itemFinalTotal,
			IsBundle:        v.Product.IsBundle,
			IsActive:        v.IsActive,
		}

		if v.Product.IsBundle {
			var totalQty float64 = 0
			for _, bi := range v.Product.BundleItems {
				totalQty += float64(bi.Quantity)
			}
			resp.QuantityValue = totalQty

			name := "products"
			shortName := "pcs"
			if lang == "uk" {
				name = "товари"
				shortName = "шт"
			}
			resp.Unit = &UnitResponse{
				ID:        DefaultBundleUnitID,
				Name:      name,
				ShortName: shortName,
			}
		} else {
			resp.QuantityValue = v.QuantityValue
			if v.UnitID != nil && v.Unit.ID != 0 {
				unitResp := &UnitResponse{ID: v.Unit.ID}
				if v.Unit.Name != nil {
					unitResp.Name = v.Unit.Name[lang]
				}
				if v.Unit.ShortName != nil {
					unitResp.ShortName = v.Unit.ShortName[lang]
				}
				resp.Unit = unitResp
			}
		}

		// Бренд
		if v.Product.Brand.ID != uuid.Nil {
			resp.BrandName = v.Product.Brand.Name
		}

		// Назва продукту
		if len(v.Product.Translations) > 0 {
			resp.Name = v.Product.Translations[0].Name
		}

		// Primary image (спершу для варіації, потім для продукту)
		for _, img := range v.Product.Images {
			if img.VariationID != nil && *img.VariationID == v.ID && img.IsPrimary {
				resp.ImageURL = img.ImageURL
				break
			}
		}
		if resp.ImageURL == "" {
			for _, img := range v.Product.Images {
				if img.VariationID == nil && img.IsPrimary {
					resp.ImageURL = img.ImageURL
					break
				}
			}
		}

		// Hover image
		for _, img := range v.Product.Images {
			if img.VariationID != nil && *img.VariationID == v.ID && img.IsHover {
				resp.HoverImageURL = img.ImageURL
				break
			}
		}
		if resp.HoverImageURL == "" {
			for _, img := range v.Product.Images {
				if img.VariationID == nil && img.IsHover {
					resp.HoverImageURL = img.ImageURL
					break
				}
			}
		}

		items = append(items, resp)
		totalCount += cartItem.Quantity
		totalPrice += itemTotal
	}

	var discountAmount int
	var originalTotalPrice = totalPrice
	if promoResult != nil {
		discountAmount = promoResult.TotalDiscountAmount
		totalPrice -= discountAmount
	}

	return CartResponse{
		Items:              items,
		TotalCount:         totalCount,
		OriginalTotalPrice: originalTotalPrice,
		DiscountAmount:     discountAmount,
		TotalPrice:         totalPrice,
		PromoCode:          promoCodeStr,
		Shipping:           shippingResp,
	}
}
