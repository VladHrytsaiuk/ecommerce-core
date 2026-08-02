//go:build legacy
// +build legacy

package http

import (
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/google/uuid"
)

const DefaultBundleUnitID = 6

// WishlistItemResponse відповідає ProductBriefResponse для єдиного формату на фронтенді
type WishlistItemResponse struct {
	ID            uuid.UUID     `json:"id"`
	ProductID     uuid.UUID     `json:"product_id"`
	Slug          string        `json:"slug"`
	SKU           string        `json:"sku"`
	Name          string        `json:"name"`
	BrandName     string        `json:"brand_name"`
	Price         int           `json:"price"`
	OldPrice      *int          `json:"old_price,omitempty"`
	ImageURL      string        `json:"image_url"`
	HoverImageURL string        `json:"hover_image_url"`
	AverageRating float64       `json:"average_rating"`
	ReviewsCount  int           `json:"reviews_count"`
	QuantityValue float64       `json:"quantity_value"`
	Unit          *UnitResponse `json:"unit,omitempty"`
	IsBundle      bool          `json:"is_bundle"`
}

// UnitResponse спрощена структура для одиниці виміру
type UnitResponse struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
}

// SyncWishlistRequest тіло запиту для синхронізації анонімного вішліста
type SyncWishlistRequest struct {
	SessionID string `json:"session_id"` // тепер опціонально, можемо брати з куки
}

// ErrorResponse стандартна структура для помилок
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// mapVariationToWishlistResponse конвертує ProductVariation у відповідь для вішліста
func mapVariationToWishlistResponse(v productDomain.ProductVariation, lang string) WishlistItemResponse {
	slug := v.Slug
	if slug == "" {
		if len(v.Product.Translations) > 0 {
			slug = v.Product.Translations[0].Slug
		}
	}
	res := WishlistItemResponse{
		ID:            v.ID,
		ProductID:     v.ProductID,
		Slug:          slug,
		SKU:           v.SKU,
		Price:         v.Price,
		OldPrice:      v.OldPrice,
		AverageRating: v.Product.AverageRating,
		ReviewsCount:  v.Product.ReviewsCount,
		IsBundle:      v.Product.IsBundle,
	}

	if v.Product.IsBundle {
		var totalQty float64 = 0
		for _, bi := range v.Product.BundleItems {
			totalQty += float64(bi.Quantity)
		}
		res.QuantityValue = totalQty

		name := "products"
		shortName := "pcs"
		if lang == "uk" {
			name = "товари"
			shortName = "шт"
		}
		res.Unit = &UnitResponse{
			ID:        DefaultBundleUnitID,
			Name:      name,
			ShortName: shortName,
		}
	} else {
		res.QuantityValue = v.QuantityValue
		if v.UnitID != nil && v.Unit.ID != 0 {
			res.Unit = &UnitResponse{
				ID:        v.Unit.ID,
				Name:      v.Unit.Name[lang],
				ShortName: v.Unit.ShortName[lang],
			}
		}
	}

	// Безпечне отримання назви бренду (може бути nil якщо бренд не прив'язаний)
	if v.Product.Brand.ID != uuid.Nil {
		res.BrandName = v.Product.Brand.Name
	}

	if len(v.Product.Translations) > 0 {
		res.Name = v.Product.Translations[0].Name
	}

	// Шукаємо primary image (спершу для варіації, потім для всього продукту)
	for _, img := range v.Product.Images {
		if img.VariationID != nil && *img.VariationID == v.ID && img.IsPrimary {
			res.ImageURL = img.ImageURL
			break
		}
	}
	if res.ImageURL == "" {
		for _, img := range v.Product.Images {
			if img.VariationID == nil && img.IsPrimary {
				res.ImageURL = img.ImageURL
				break
			}
		}
	}

	// Шукаємо hover image
	for _, img := range v.Product.Images {
		if img.VariationID != nil && *img.VariationID == v.ID && img.IsHover {
			res.HoverImageURL = img.ImageURL
			break
		}
	}
	if res.HoverImageURL == "" {
		for _, img := range v.Product.Images {
			if img.VariationID == nil && img.IsHover {
				res.HoverImageURL = img.ImageURL
				break
			}
		}
	}

	return res
}

// mapVariationsToWishlistResponse конвертує масив варіацій у відповіді для вішліста
func mapVariationsToWishlistResponse(variations []productDomain.ProductVariation, lang string) []WishlistItemResponse {
	res := make([]WishlistItemResponse, len(variations))
	for i, v := range variations {
		res[i] = mapVariationToWishlistResponse(v, lang)
	}
	return res
}
