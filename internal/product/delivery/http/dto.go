package http

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
)

type ProductResponse struct {
	ID                uuid.UUID                  `json:"id"`
	Slug              string                     `json:"slug"`
	Slugs             map[string]string          `json:"slugs,omitempty"`
	BrandID           uuid.UUID                  `json:"brand_id"`
	BrandName         string                     `json:"brand_name"`
	CategoryID        uuid.UUID                  `json:"category_id"`
	CategoryName      string                     `json:"category_name"`
	Name              string                     `json:"name"`
	Description       string                     `json:"description"`
	UsageInstructions string                     `json:"usage_instructions"`
	MetaTitle         string                     `json:"meta_title,omitempty"`
	MetaDescription   string                     `json:"meta_description,omitempty"`
	MetaKeywords      string                     `json:"meta_keywords,omitempty"`
	AverageRating     float64                    `json:"average_rating"`
	ReviewsCount      int                        `json:"reviews_count"`
	IsActive          bool                       `json:"is_active"`
	IsRecommended     bool                       `json:"is_recommended"`
	IsBundle          bool                       `json:"is_bundle"`
	PriceStrategy     string                     `json:"price_strategy,omitempty"`
	Variations        []ProductVariationResponse `json:"variations"`
	Images            []ProductImageResponse     `json:"images"`
	Badges            []BadgeResponse            `json:"badges"`
	Breadcrumbs       []BreadcrumbResponse       `json:"breadcrumbs,omitempty"`
	BundleItems       []BundleItemResponse       `json:"bundle_items,omitempty"`
}

type BreadcrumbResponse struct {
	ID    uuid.UUID         `json:"id"`
	Slug  string            `json:"slug"`
	Slugs map[string]string `json:"slugs,omitempty"`
	Name  string            `json:"name"`
}

type ProductBriefResponse struct {
	ID            uuid.UUID         `json:"id"`
	ProductID     uuid.UUID         `json:"product_id"`
	Slug          string            `json:"slug"`
	Slugs         map[string]string `json:"slugs,omitempty"`
	SKU           string            `json:"sku"`
	Name          string            `json:"name"`
	BrandID       uuid.UUID         `json:"brand_id"`
	BrandName     string            `json:"brand_name"`
	Price         int               `json:"price"`
	OldPrice      *int              `json:"old_price,omitempty"`
	ImageURL      string            `json:"image_url"`
	HoverImageURL string            `json:"hover_image_url"`
	AverageRating float64           `json:"average_rating"`
	ReviewsCount  int               `json:"reviews_count"`
	QuantityValue float64           `json:"quantity_value"`
	Unit          *UnitResponse     `json:"unit,omitempty"`
	Badges        []BadgeResponse   `json:"badges"`
	IsBundle      bool              `json:"is_bundle"`
	IsRecommended bool              `json:"is_recommended"`
}

type GetProductsRequest struct {
	pagination.Params
	CategoryID     []string `form:"category_id"`
	BrandID        []string `form:"brand_id"`
	BrandSlug      []string `form:"brand_slug"`
	MinPrice       *int     `form:"min_price"`
	MaxPrice       *int     `form:"max_price"`
	QuantityValues []string `form:"quantity_values"`
	Q              string   `form:"q"` // Search query
	// AttrValues will be parsed manually from query since it's a dynamic map
}

type GetProductFiltersRequest struct {
	CategoryID     []string `form:"category_id"`
	BrandID        []string `form:"brand_id"`
	BrandSlug      []string `form:"brand_slug"`
	MinPrice       *int     `form:"min_price"`
	MaxPrice       *int     `form:"max_price"`
	QuantityValues []string `form:"quantity_values"`
}

type UnitResponse struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
}

type CharacteristicResponse struct {
	AttributeValueID *uuid.UUID    `json:"attribute_value_id"`
	AttributeID      int           `json:"attribute_id"`
	Code             string        `json:"code"`
	Name             string        `json:"name"`
	ValueString      *string       `json:"value_string"`
	ValueNumeric     *float64      `json:"value_numeric"`
	Unit             *UnitResponse `json:"unit"`
	SortOrder        int           `json:"sort_order"`
	IsFilterable     bool          `json:"is_filterable"`
}

type ProductVariationResponse struct {
	ID              uuid.UUID                `json:"id"`
	Slug            string                   `json:"slug"`
	Name            string                   `json:"name"` // власна назва варіації або (fallback) назва товару
	SKU             string                   `json:"sku"`
	Barcode         string                   `json:"barcode"`
	Price           int                      `json:"price"`
	OldPrice        *int                     `json:"old_price,omitempty"`
	QuantityValue   float64                  `json:"quantity_value"`
	Weight          float64                  `json:"weight"`
	Unit            *UnitResponse            `json:"unit"`
	Characteristics []CharacteristicResponse `json:"characteristics"`
	Images          []ProductImageResponse   `json:"images"`
	Badges          []BadgeResponse          `json:"badges"`
}

type BadgeResponse struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	ColorHex string `json:"color_hex"`
}

type ProductImageResponse struct {
	ID        uuid.UUID `json:"id"`
	ImageURL  string    `json:"image_url"`
	IsPrimary bool      `json:"is_primary"`
	IsHover   bool      `json:"is_hover"`
	SortOrder int       `json:"sort_order"`
	AltText   string    `json:"alt_text,omitempty"`
}

type BundleItemResponse struct {
	VariationID     uuid.UUID                `json:"variation_id"`
	Quantity        int                      `json:"quantity"`
	ProductID       uuid.UUID                `json:"product_id"`
	ProductName     string                   `json:"product_name"`
	ProductSlug     string                   `json:"product_slug"`
	SKU             string                   `json:"sku"`
	Price           int                      `json:"price"`
	QuantityValue   float64                  `json:"quantity_value"`
	ImageURL        string                   `json:"image_url"`
	Unit            *UnitResponse            `json:"unit,omitempty"`
	Characteristics []CharacteristicResponse `json:"characteristics"`
}

type BrandResponse struct {
	ID    uuid.UUID `json:"id"`
	Slug  string    `json:"slug"`
	Name  string    `json:"name"`
	Count int       `json:"count,omitempty"`
}

type ProductReviewResponse struct {
	ID            uuid.UUID  `json:"id"`
	ProductID     uuid.UUID  `json:"product_id,omitempty"`
	ProductName   string     `json:"product_name,omitempty"`
	ProductSlug   string     `json:"product_slug,omitempty"`
	UserID        uuid.UUID  `json:"user_id"`
	UserFirstName string     `json:"user_first_name,omitempty"`
	UserLastName  string     `json:"user_last_name,omitempty"`
	ParentID      *uuid.UUID `json:"parent_id"`
	Rating        *int       `json:"rating"` // null for replies (parent_id != nil)
	Comment       string     `json:"comment"`
	Status        string     `json:"status"`
	RejectReason  *string    `json:"reject_reason,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type RejectReviewRequest struct {
	Reason *string `json:"reason,omitempty"`
}

type FilterDiscoveryResponse struct {
	Price      PriceFilterDTO                `json:"price"`
	Brands     []BrandResponse               `json:"brands"`
	Quantities map[string][]QuantityValueDTO `json:"quantities"`
	Attributes []AttributeFilterDTO          `json:"attributes"`
}

type QuantityValueDTO struct {
	Value  float64 `json:"value"`
	UnitID int     `json:"unit_id"`
	Count  int     `json:"count"`
}

type PriceFilterDTO struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type AttributeFilterDTO struct {
	ID     int            `json:"id"`
	Code   string         `json:"code"`
	Name   string         `json:"name"`
	Values []AttrValueDTO `json:"values"`
}

type AttrValueDTO struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type CreateReviewRequest struct {
	Rating   *int       `json:"rating" binding:"omitempty,min=1,max=5"` // optional for replies
	Comment  string     `json:"comment" binding:"required,max=2000"`
	ParentID *uuid.UUID `json:"parent_id,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type QuickSearchResponse struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Slug     string    `json:"slug"`
	SKU      string    `json:"sku"`
	Price    int       `json:"price"`
	OldPrice *int      `json:"old_price,omitempty"`
	ImageURL string    `json:"image_url"`
}

func mapQuickSearchToResponse(p domain.QuickSearchProduct) QuickSearchResponse {
	return QuickSearchResponse{
		ID:       p.ID,
		Name:     p.Name,
		Slug:     p.Slug,
		SKU:      p.SKU,
		Price:    p.Price,
		OldPrice: p.OldPrice,
		ImageURL: p.ImageURL,
	}
}

func mapProductToResponse(p domain.Product, lang string, requestedSlug string, breadcrumbs []BreadcrumbResponse) ProductResponse {
	categoryName := ""
	for _, t := range p.Category.Translations {
		if t.LanguageCode == lang {
			categoryName = t.Name
			break
		}
	}
	if categoryName == "" && len(p.Category.Translations) > 0 {
		categoryName = p.Category.Translations[0].Name
	}

	productSlug := requestedSlug
	productName := ""
	slugsMap := make(map[string]string)

	var currentLangTranslation *domain.ProductTranslation
	for i, t := range p.Translations {
		slugsMap[t.LanguageCode] = t.Slug
		if t.LanguageCode == lang {
			currentLangTranslation = &p.Translations[i]
		}
	}
	if currentLangTranslation == nil && len(p.Translations) > 0 {
		currentLangTranslation = &p.Translations[0]
	}

	if currentLangTranslation != nil {
		productName = currentLangTranslation.Name
		if productSlug == "" {
			productSlug = currentLangTranslation.Slug
		}
	}

	fullBreadcrumbs := make([]BreadcrumbResponse, len(breadcrumbs), len(breadcrumbs)+1)
	copy(fullBreadcrumbs, breadcrumbs)

	// Add the product itself as the last element
	fullBreadcrumbs = append(fullBreadcrumbs, BreadcrumbResponse{
		ID:    p.ID,
		Slug:  productSlug,
		Slugs: slugsMap,
		Name:  productName,
	})

	res := ProductResponse{
		ID:            p.ID,
		BrandID:       p.BrandID,
		BrandName:     p.Brand.Name,
		CategoryID:    p.CategoryID,
		CategoryName:  categoryName,
		AverageRating: p.AverageRating,
		ReviewsCount:  p.ReviewsCount,
		IsActive:      p.IsActive,
		IsRecommended: p.IsRecommended,
		IsBundle:      p.IsBundle,
		Variations:    make([]ProductVariationResponse, 0, len(p.Variations)),
		Images:        make([]ProductImageResponse, 0),
		Breadcrumbs:   fullBreadcrumbs,
		Slug:          productSlug,
		Slugs:         slugsMap,
	}

	if currentLangTranslation != nil {
		res.Name = currentLangTranslation.Name
		res.Slug = currentLangTranslation.Slug
		res.Description = currentLangTranslation.Description
		res.UsageInstructions = currentLangTranslation.UsageInstructions
		res.MetaTitle = currentLangTranslation.MetaTitle
		res.MetaDescription = currentLangTranslation.MetaDescription
		res.MetaKeywords = currentLangTranslation.MetaKeywords
	}

	// 1. Спільні характеристики (Common) — реальні атрибути з БД
	var commonCharacteristics []CharacteristicResponse
	if p.IsBundle {
		var countAV, itemsAV *domain.AttributeValue
		for i := range p.AttributeValues {
			av := &p.AttributeValues[i]
			if av.Attribute.Code == "bundle_items_count" {
				countAV = av
			} else if av.Attribute.Code == "bundle_items" {
				itemsAV = av
			}
		}

		commonCharacteristics = make([]CharacteristicResponse, 0, 3)

		// 1. Кількість товарів у наборі
		if countAV != nil {
			mapped := mapAttributesToCharacteristics([]domain.AttributeValue{*countAV}, lang)
			if len(mapped) > 0 {
				mapped[0].SortOrder = 10
				commonCharacteristics = append(commonCharacteristics, mapped[0])
			}
		} else {
			var totalQty float64 = 0
			for _, bi := range p.BundleItems {
				totalQty += float64(bi.Quantity)
			}
			commonCharacteristics = append(commonCharacteristics, CharacteristicResponse{
				AttributeValueID: nil,
				AttributeID:      0,
				Code:             "bundle_items_count",
				Name:             getLocalizedName("Кількість товарів у наборі", "Number of items in the set", lang),
				ValueNumeric:     &totalQty,
				Unit: &UnitResponse{
					ID:        6,
					Name:      getLocalizedName("товари", "products", lang),
					ShortName: getLocalizedName("шт", "pcs", lang),
				},
				SortOrder:    10,
				IsFilterable: false,
			})
		}

		// 2. Самі товари (компоненти)
		if itemsAV != nil {
			mapped := mapAttributesToCharacteristics([]domain.AttributeValue{*itemsAV}, lang)
			if len(mapped) > 0 {
				mapped[0].SortOrder = 20
				commonCharacteristics = append(commonCharacteristics, mapped[0])
			}
		} else {
			var itemsList []string
			for _, bi := range p.BundleItems {
				var prodName string
				if len(bi.Variation.Product.Translations) > 0 {
					prodName = bi.Variation.Product.Translations[0].Name
				} else {
					prodName = bi.Variation.Product.ID.String()
				}

				if lang == "uk" {
					itemsList = append(itemsList, fmt.Sprintf("%s (%d шт.)", prodName, bi.Quantity))
				} else {
					itemsList = append(itemsList, fmt.Sprintf("%s (%d pcs)", prodName, bi.Quantity))
				}
			}
			itemsStr := strings.Join(itemsList, "\n")

			commonCharacteristics = append(commonCharacteristics, CharacteristicResponse{
				AttributeValueID: nil,
				AttributeID:      0,
				Code:             "bundle_items",
				Name:             getLocalizedName("Товари у наборі", "Items in the set", lang),
				ValueString:      &itemsStr,
				SortOrder:        20,
				IsFilterable:     false,
			})
		}

		// 3. Вага набору (беремо вже розраховану та заокруглену вагу з першої варіації набору)
		var bundleWeight float64 = 0
		if len(p.Variations) > 0 {
			bundleWeight = p.Variations[0].Weight
		}
		commonCharacteristics = append(commonCharacteristics, CharacteristicResponse{
			AttributeValueID: nil,
			AttributeID:      0,
			Code:             "weight",
			Name:             getLocalizedName("Вага", "Weight", lang),
			ValueNumeric:     &bundleWeight,
			Unit: &UnitResponse{
				ID:        4,
				Name:      getLocalizedName("кілограми", "kilograms", lang),
				ShortName: getLocalizedName("кг", "kg", lang),
			},
			SortOrder:    30,
			IsFilterable: true,
		})
	} else {
		commonAttrs := make([]domain.AttributeValue, 0)
		for _, av := range p.AttributeValues {
			if !av.Attribute.IsVariantSpecific {
				commonAttrs = append(commonAttrs, av)
			}
		}
		commonCharacteristics = mapAttributesToCharacteristics(commonAttrs, lang)

		// Додаємо синтезований Бренд (SortOrder = 10, attribute_value_id = nil)
		brandName := p.Brand.Name
		commonCharacteristics = append([]CharacteristicResponse{
			{
				AttributeValueID: nil,
				AttributeID:      0,
				Code:             "brand",
				Name:             getLocalizedName("Бренд", "Brand", lang),
				ValueString:      &brandName,
				SortOrder:        10,
				IsFilterable:     true,
			},
		}, commonCharacteristics...)
	}

	// Сортуємо загальні характеристики за sort_order
	sort.Slice(commonCharacteristics, func(i, j int) bool {
		return commonCharacteristics[i].SortOrder < commonCharacteristics[j].SortOrder
	})

	// 2. Варіації — barcode, quantity, unit як окремі поля + специфічні атрибути
	for _, v := range p.Variations {
		variantAttrs := make([]domain.AttributeValue, 0)
		for _, av := range v.AttributeValues {
			if av.Attribute.IsVariantSpecific {
				variantAttrs = append(variantAttrs, av)
			}
		}

		// Сира власна назва варіації саме для цієї мови (порожня, якщо власної немає).
		// Без fallback на назву товару — щоб адмін-форма коректно префілила (порожнє поле
		// = немає власної назви), а не «зашивала» назву товару у варіацію.
		vRes := ProductVariationResponse{
			ID:            v.ID,
			Slug:          v.Slug,
			Name:          v.Name[lang],
			SKU:           v.SKU,
			Barcode:       v.Barcode,
			Price:         v.Price,
			OldPrice:      v.OldPrice,
			QuantityValue: v.QuantityValue,
			Weight:        v.Weight,
			Images:        make([]ProductImageResponse, 0),
		}

		variationCharacteristics := mapAttributesToCharacteristics(variantAttrs, lang)

		var general []domain.ProductImage
		var own []domain.ProductImage

		for _, img := range p.Images {
			if img.VariationID == nil {
				general = append(general, img)
			} else if *img.VariationID == v.ID {
				own = append(own, img)
			}
		}

		var primary *domain.ProductImage
		var hover *domain.ProductImage

		for i := range own {
			if own[i].IsPrimary && primary == nil {
				primary = &own[i]
			}
			if own[i].IsHover && hover == nil {
				hover = &own[i]
			}
		}

		for i := range general {
			if general[i].IsPrimary && primary == nil {
				primary = &general[i]
			}
			if general[i].IsHover && hover == nil {
				hover = &general[i]
			}
		}

		var ownAdditional []domain.ProductImage
		var generalAdditional []domain.ProductImage

		for i := range own {
			if own[i].IsPrimary || own[i].IsHover {
				continue
			}
			ownAdditional = append(ownAdditional, own[i])
		}

		for i := range general {
			if general[i].IsPrimary || general[i].IsHover {
				continue
			}
			generalAdditional = append(generalAdditional, general[i])
		}

		additional := generalAdditional
		if len(ownAdditional) > 0 {
			additional = ownAdditional
		}

		sort.Slice(additional, func(i, j int) bool {
			return additional[i].SortOrder < additional[j].SortOrder
		})

		vRes.Images = make([]ProductImageResponse, 0)
		added := make(map[uuid.UUID]bool)

		addImg := func(img domain.ProductImage) {
			if added[img.ID] {
				return
			}
			isP := false
			isH := false
			if primary != nil && primary.ID == img.ID {
				isP = true
			}
			if hover != nil && hover.ID == img.ID {
				isH = true
			}

			vRes.Images = append(vRes.Images, ProductImageResponse{
				ID:        img.ID,
				ImageURL:  img.ImageURL,
				IsPrimary: isP,
				IsHover:   isH,
				SortOrder: img.SortOrder,
				AltText:   img.AltText[lang],
			})
			added[img.ID] = true
		}

		if primary != nil {
			addImg(*primary)
		}
		if hover != nil {
			addImg(*hover)
		}
		for _, img := range additional {
			addImg(img)
		}

		// Unit варіації як структурований об'єкт
		if v.UnitID != nil && v.Unit.ID != 0 {
			vRes.Unit = &UnitResponse{
				ID:        v.Unit.ID,
				Name:      getLocalized(v.Unit.Name, lang),
				ShortName: getLocalized(v.Unit.ShortName, lang),
			}
		}

		// Синтезуємо EAN/Штрихкод з поля Barcode варіації, якщо його немає в списку характеристик
		if v.Barcode != "" {
			hasBarcode := false
			for _, char := range variationCharacteristics {
				c := strings.ToLower(char.Code)
				n := strings.ToLower(char.Name)
				if c == "ean" || c == "barcode" || strings.Contains(n, "ean") || strings.Contains(n, "штрих") {
					hasBarcode = true
					break
				}
			}
			if !hasBarcode {
				barcodeVal := v.Barcode
				variationCharacteristics = append(variationCharacteristics, CharacteristicResponse{
					AttributeValueID: nil,
					AttributeID:      0,
					Code:             "ean",
					Name:             getLocalizedName("EAN-код", "EAN-code", lang),
					ValueString:      &barcodeVal,
					SortOrder:        99,
					IsFilterable:     false,
				})
			}
		}

		// Синтезуємо Вагу з поля Weight варіації, якщо її немає в списку характеристик і це НЕ набір
		if v.Weight > 0 && !p.IsBundle {
			hasWeight := false
			for _, char := range variationCharacteristics {
				c := strings.ToLower(char.Code)
				n := strings.ToLower(char.Name)
				if c == "weight" || strings.Contains(n, "вага") || strings.Contains(n, "weight") {
					hasWeight = true
					break
				}
			}
			if !hasWeight {
				weightVal := v.Weight
				weightUnit := &UnitResponse{
					ID:        4,
					Name:      getLocalizedName("кілограми", "kilograms", lang),
					ShortName: getLocalizedName("кг", "kg", lang),
				}
				variationCharacteristics = append(variationCharacteristics, CharacteristicResponse{
					AttributeValueID: nil,
					AttributeID:      0,
					Code:             "weight",
					Name:             getLocalizedName("Вага", "Weight", lang),
					ValueString:      nil,
					ValueNumeric:     &weightVal,
					Unit:             weightUnit,
					SortOrder:        50,
					IsFilterable:     true,
				})
			}
		}

		// Синтезуємо Кількість з поля QuantityValue варіації, якщо її немає в списку характеристик
		if v.QuantityValue > 0 {
			hasQty := false
			for _, char := range variationCharacteristics {
				c := strings.ToLower(char.Code)
				n := strings.ToLower(char.Name)
				if c == "quantity" || c == "count" || strings.Contains(n, "кількість") || strings.Contains(n, "кол-во") {
					hasQty = true
					break
				}
			}
			if !hasQty {
				var unitResp *UnitResponse
				if v.UnitID != nil && v.Unit.ID != 0 {
					unitResp = &UnitResponse{
						ID:        v.Unit.ID,
						Name:      getLocalized(v.Unit.Name, lang),
						ShortName: getLocalized(v.Unit.ShortName, lang),
					}
				}
				qtyVal := v.QuantityValue
				variationCharacteristics = append(variationCharacteristics, CharacteristicResponse{
					AttributeValueID: nil,
					AttributeID:      0,
					Code:             "quantity",
					Name:             getLocalizedName("Кількість", "Quantity", lang),
					ValueString:      nil,
					ValueNumeric:     &qtyVal,
					Unit:             unitResp,
					SortOrder:        40,
					IsFilterable:     true,
				})
			}
		}

		// Сортуємо характеристики варіації
		sort.Slice(variationCharacteristics, func(i, j int) bool {
			return variationCharacteristics[i].SortOrder < variationCharacteristics[j].SortOrder
		})

		// Створюємо єдиний відсортований масив характеристик (загальні + специфічні)
		vRes.Characteristics = make([]CharacteristicResponse, 0, len(commonCharacteristics)+len(variationCharacteristics))
		vRes.Characteristics = append(vRes.Characteristics, commonCharacteristics...)
		vRes.Characteristics = append(vRes.Characteristics, variationCharacteristics...)
		sort.Slice(vRes.Characteristics, func(x, y int) bool {
			return vRes.Characteristics[x].SortOrder < vRes.Characteristics[y].SortOrder
		})

		// Бейджі варіації: об'єднуємо бейджі товару + бейджі варіації + computed badges, без дублікатів
		vRes.Badges = mergeAndDeduplicateBadges(p.Badges, v.Badges, v.ComputedBadges, lang)

		res.Variations = append(res.Variations, vRes)
	}

	// Сортуємо варіації: пріоритет варіації з requestedSlug, потім за QuantityValue ASC, потім за Price ASC
	sort.Slice(res.Variations, func(i, j int) bool {
		if requestedSlug != "" {
			if res.Variations[i].Slug == requestedSlug {
				return true
			}
			if res.Variations[j].Slug == requestedSlug {
				return false
			}
		}
		if res.Variations[i].QuantityValue != res.Variations[j].QuantityValue {
			return res.Variations[i].QuantityValue < res.Variations[j].QuantityValue
		}
		return res.Variations[i].Price < res.Variations[j].Price
	})

	// Вітрина: коли товар відкрито за slug конкретної варіації і та має власну назву —
	// показуємо її як назву товару (кожна варіація = окремо названий товар).
	// Лише для запиту за slug; для admin GET-by-id (requestedSlug == "") лишаємо назву товару,
	// інакше форма редагування затерла б назву товару назвою варіації.
	if requestedSlug != "" && len(res.Variations) > 0 &&
		res.Variations[0].Slug == requestedSlug && res.Variations[0].Name != "" {
		res.Name = res.Variations[0].Name
	}

	// 3. Загальні фото товару (де variation_id == NULL)
	for _, img := range p.Images {
		if img.VariationID == nil {
			res.Images = append(res.Images, ProductImageResponse{
				ID:        img.ID,
				ImageURL:  img.ImageURL,
				IsPrimary: img.IsPrimary,
				IsHover:   img.IsHover,
				SortOrder: img.SortOrder,
				AltText:   img.AltText[lang],
			})
		}
	}

	// 4. Бейджі товару (product-level) — ручні + computed, без дублікатів
	res.Badges = mergeAndDeduplicateBadges(p.Badges, nil, p.ComputedBadges, lang)

	// 5. Компоненти набору
	if p.IsBundle {
		res.PriceStrategy = p.PriceStrategy
		res.BundleItems = make([]BundleItemResponse, 0, len(p.BundleItems))
		for _, bi := range p.BundleItems {
			item := BundleItemResponse{
				VariationID:   bi.VariationID,
				Quantity:      bi.Quantity,
				ProductID:     bi.Variation.ProductID,
				SKU:           bi.Variation.SKU,
				Price:         bi.Variation.Price,
				QuantityValue: bi.Variation.QuantityValue,
			}
			// Назва та slug з продукту компонента
			if bi.Variation.Product.ID != uuid.Nil {
				if len(bi.Variation.Product.Translations) > 0 {
					item.ProductName = bi.Variation.Product.Translations[0].Name
					item.ProductSlug = bi.Variation.Product.Translations[0].Slug
				}
				// Перше фото компонента
				for _, img := range bi.Variation.Product.Images {
					if img.IsPrimary {
						item.ImageURL = img.ImageURL
						break
					}
				}
				if item.ImageURL == "" && len(bi.Variation.Product.Images) > 0 {
					item.ImageURL = bi.Variation.Product.Images[0].ImageURL
				}
			}
			// Unit варіації
			if bi.Variation.UnitID != nil && bi.Variation.Unit.ID != 0 {
				item.Unit = &UnitResponse{
					ID:        bi.Variation.Unit.ID,
					Name:      getLocalized(bi.Variation.Unit.Name, lang),
					ShortName: getLocalized(bi.Variation.Unit.ShortName, lang),
				}
			}

			// Характеристики компонента набору (загальні + специфічні для варіації + бренд)
			compAttrs := make([]domain.AttributeValue, 0)
			if bi.Variation.Product.ID != uuid.Nil {
				for _, av := range bi.Variation.Product.AttributeValues {
					if !av.Attribute.IsVariantSpecific {
						compAttrs = append(compAttrs, av)
					}
				}
			}
			for _, av := range bi.Variation.AttributeValues {
				if av.Attribute.IsVariantSpecific {
					compAttrs = append(compAttrs, av)
				}
			}

			item.Characteristics = mapAttributesToCharacteristics(compAttrs, lang)

			// Додаємо синтезований Бренд для компонента
			if bi.Variation.Product.ID != uuid.Nil && bi.Variation.Product.Brand.Name != "" {
				brandVal := bi.Variation.Product.Brand.Name
				item.Characteristics = append([]CharacteristicResponse{
					{
						AttributeValueID: nil,
						AttributeID:      0,
						Code:             "brand",
						Name:             getLocalizedName("Бренд", "Brand", lang),
						ValueString:      &brandVal,
						SortOrder:        10,
						IsFilterable:     true,
					},
				}, item.Characteristics...)
			}

			// Синтезуємо EAN/Штрихкод з поля Barcode варіації компонента
			if bi.Variation.Barcode != "" {
				hasBarcode := false
				for _, char := range item.Characteristics {
					c := strings.ToLower(char.Code)
					n := strings.ToLower(char.Name)
					if c == "ean" || c == "barcode" || strings.Contains(n, "ean") || strings.Contains(n, "штрих") {
						hasBarcode = true
						break
					}
				}
				if !hasBarcode {
					barcodeVal := bi.Variation.Barcode
					item.Characteristics = append(item.Characteristics, CharacteristicResponse{
						AttributeValueID: nil,
						AttributeID:      0,
						Code:             "ean",
						Name:             getLocalizedName("EAN-код", "EAN-code", lang),
						ValueString:      &barcodeVal,
						SortOrder:        90,
						IsFilterable:     false,
					})
				}
			}

			// Синтезуємо Вагу з поля Weight варіації компонента
			if bi.Variation.Weight > 0 {
				hasWeight := false
				for _, char := range item.Characteristics {
					c := strings.ToLower(char.Code)
					n := strings.ToLower(char.Name)
					if c == "weight" || strings.Contains(n, "вага") || strings.Contains(n, "weight") {
						hasWeight = true
						break
					}
				}
				if !hasWeight {
					weightVal := bi.Variation.Weight
					weightUnit := &UnitResponse{
						ID:        4,
						Name:      getLocalizedName("кілограми", "kilograms", lang),
						ShortName: getLocalizedName("кг", "kg", lang),
					}
					item.Characteristics = append(item.Characteristics, CharacteristicResponse{
						AttributeValueID: nil,
						AttributeID:      0,
						Code:             "weight",
						Name:             getLocalizedName("Вага", "Weight", lang),
						ValueString:      nil,
						ValueNumeric:     &weightVal,
						Unit:             weightUnit,
						SortOrder:        50,
						IsFilterable:     true,
					})
				}
			}

			// Синтезуємо Кількість з поля QuantityValue варіації компонента
			if bi.Variation.QuantityValue > 0 {
				hasQty := false
				for _, char := range item.Characteristics {
					c := strings.ToLower(char.Code)
					n := strings.ToLower(char.Name)
					if c == "quantity" || c == "count" || strings.Contains(n, "кількість") || strings.Contains(n, "кол-во") {
						hasQty = true
						break
					}
				}
				if !hasQty {
					var unitResp *UnitResponse
					if bi.Variation.UnitID != nil && bi.Variation.Unit.ID != 0 {
						unitResp = &UnitResponse{
							ID:        bi.Variation.Unit.ID,
							Name:      getLocalized(bi.Variation.Unit.Name, lang),
							ShortName: getLocalized(bi.Variation.Unit.ShortName, lang),
						}
					}
					qtyVal := bi.Variation.QuantityValue
					item.Characteristics = append(item.Characteristics, CharacteristicResponse{
						AttributeValueID: nil,
						AttributeID:      0,
						Code:             "quantity",
						Name:             getLocalizedName("Кількість", "Quantity", lang),
						ValueString:      nil,
						ValueNumeric:     &qtyVal,
						Unit:             unitResp,
						SortOrder:        40,
						IsFilterable:     true,
					})
				}
			}

			// Сортуємо характеристики за sort_order
			sort.Slice(item.Characteristics, func(x, y int) bool {
				return item.Characteristics[x].SortOrder < item.Characteristics[y].SortOrder
			})

			res.BundleItems = append(res.BundleItems, item)
		}
	}

	return res
}

func getLocalizedName(uk, en, lang string) string {
	if lang == "uk" {
		return uk
	}
	return en
}

func getLocalized(m map[string]string, lang string) string {
	if m == nil {
		return ""
	}
	if val, ok := m[lang]; ok {
		return val
	}
	// Fallback to uk then en
	if val, ok := m["uk"]; ok {
		return val
	}
	if val, ok := m["en"]; ok {
		return val
	}
	return ""
}

func mapAttributesToCharacteristics(attrs []domain.AttributeValue, lang string) []CharacteristicResponse {
	if len(attrs) == 0 {
		return []CharacteristicResponse{}
	}

	res := make([]CharacteristicResponse, 0, len(attrs))
	for _, attr := range attrs {
		dto := CharacteristicResponse{
			AttributeValueID: &attr.ID,
			AttributeID:      attr.AttributeID,
			Code:             attr.Attribute.Code,
			SortOrder:        attr.Attribute.SortOrder,
			IsFilterable:     attr.Attribute.IsFilterable,
		}

		// Знаходимо переклад назви атрибута ("Вага", "Колір")
		for _, t := range attr.Attribute.Translations {
			if t.LanguageCode == lang {
				dto.Name = t.Name
				break
			}
			if t.LanguageCode == "uk" && dto.Name == "" {
				dto.Name = t.Name
			}
		}

		// Розділяємо значення: value_string та value_numeric окремо
		if attr.ValueNumeric != nil {
			dto.ValueNumeric = attr.ValueNumeric
		}
		if attr.ValueString != nil {
			localizedVal := getLocalized(attr.ValueString, lang)
			if localizedVal != "" {
				if attr.Attribute.Code == "bundle_items" {
					localizedVal = strings.ReplaceAll(localizedVal, ", ", "\n")
				}
				dto.ValueString = &localizedVal
			}
		}

		// Одиниця виміру як структурований об'єкт (пріоритет: value-level, потім attribute-level)
		if attr.Unit != nil && attr.Unit.ID != 0 {
			dto.Unit = &UnitResponse{
				ID:        attr.Unit.ID,
				Name:      getLocalized(attr.Unit.Name, lang),
				ShortName: getLocalized(attr.Unit.ShortName, lang),
			}
		} else if attr.Attribute.Unit != nil && attr.Attribute.Unit.ID != 0 {
			dto.Unit = &UnitResponse{
				ID:        attr.Attribute.Unit.ID,
				Name:      getLocalized(attr.Attribute.Unit.Name, lang),
				ShortName: getLocalized(attr.Attribute.Unit.ShortName, lang),
			}
		}

		res = append(res, dto)
	}
	return res
}

func mapReviewToResponse(r domain.ProductReview) ProductReviewResponse {
	res := ProductReviewResponse{
		ID:            r.ID,
		ProductID:     r.ProductID,
		ProductName:   r.ProductName,
		ProductSlug:   r.ProductSlug,
		UserID:        r.UserID,
		UserFirstName: r.UserFirstName,
		UserLastName:  r.UserLastName,
		ParentID:      r.ParentID,
		Comment:       r.Comment,
		Status:        r.Status,
		RejectReason:  r.RejectReason,
		CreatedAt:     r.CreatedAt,
	}
	// Replies (parent_id != nil) do not have a rating — return null
	if r.ParentID == nil {
		res.Rating = &r.Rating
	}
	return res
}

func mapReviewListToResponse(reviews []domain.ProductReview) []ProductReviewResponse {
	res := make([]ProductReviewResponse, len(reviews))
	for i, r := range reviews {
		res[i] = mapReviewToResponse(r)
	}
	return res
}

func mapBrandListToResponse(brands []domain.Brand) []BrandResponse {
	res := make([]BrandResponse, len(brands))
	for i, b := range brands {
		res[i] = BrandResponse{
			ID:   b.ID,
			Slug: b.Slug,
			Name: b.Name,
		}
	}
	return res
}

func mapFilterDiscoveryToResponse(fd domain.FilterDiscovery, lang string) FilterDiscoveryResponse {
	res := FilterDiscoveryResponse{
		Price: PriceFilterDTO{
			Min: fd.MinPrice,
			Max: fd.MaxPrice,
		},
		Brands:     make([]BrandResponse, 0, len(fd.Brands)),
		Attributes: make([]AttributeFilterDTO, 0, len(fd.Attributes)),
	}

	for _, b := range fd.Brands {
		res.Brands = append(res.Brands, BrandResponse{
			ID:    b.ID,
			Slug:  b.Slug,
			Name:  b.Name,
			Count: b.Count,
		})
	}

	groupedQuantities := make(map[string][]QuantityValueDTO)
	for _, q := range fd.Quantities {
		unit := q.Unit
		if unit == "" {
			unit = "-"
		}
		groupedQuantities[unit] = append(groupedQuantities[unit], QuantityValueDTO{
			Value:  q.Value,
			UnitID: q.UnitID,
			Count:  q.Count,
		})
	}

	for unit := range groupedQuantities {
		sort.Slice(groupedQuantities[unit], func(i, j int) bool {
			return groupedQuantities[unit][i].Value < groupedQuantities[unit][j].Value
		})
	}
	res.Quantities = groupedQuantities

	for _, attr := range fd.Attributes {
		attrDTO := AttributeFilterDTO{
			ID:     attr.ID,
			Code:   attr.Code,
			Name:   attr.Name,
			Values: make([]AttrValueDTO, 0, len(attr.Values)),
		}
		for _, v := range attr.Values {
			attrDTO.Values = append(attrDTO.Values, AttrValueDTO{
				Code:  v.Code,
				Label: v.Label,
				Count: v.Count,
			})
		}
		res.Attributes = append(res.Attributes, attrDTO)
	}

	return res
}
func mapVariationToBriefResponse(v domain.ProductVariation, lang string) ProductBriefResponse {
	slugsMap := make(map[string]string)
	var currentLangTranslation *domain.ProductTranslation

	for i, t := range v.Product.Translations {
		slugsMap[t.LanguageCode] = t.Slug
		if t.LanguageCode == lang {
			currentLangTranslation = &v.Product.Translations[i]
		}
	}
	if currentLangTranslation == nil && len(v.Product.Translations) > 0 {
		currentLangTranslation = &v.Product.Translations[0]
	}

	slug := v.Slug
	if slug == "" && currentLangTranslation != nil {
		slug = currentLangTranslation.Slug
	}

	res := ProductBriefResponse{
		ID:            v.ID,
		ProductID:     v.ProductID,
		Slug:          slug,
		Slugs:         slugsMap,
		SKU:           v.SKU,
		BrandID:       v.Product.BrandID,
		BrandName:     v.Product.Brand.Name,
		Price:         v.Price,
		OldPrice:      v.OldPrice,
		AverageRating: v.Product.AverageRating,
		ReviewsCount:  v.Product.ReviewsCount,
		IsBundle:      v.Product.IsBundle,
		IsRecommended: v.Product.IsRecommended,
	}

	if v.Product.IsBundle {
		var totalQty float64 = 0
		for _, bi := range v.Product.BundleItems {
			totalQty += float64(bi.Quantity)
		}
		res.QuantityValue = totalQty
		res.Unit = &UnitResponse{
			ID:        6,
			Name:      getLocalizedName("товари", "products", lang),
			ShortName: getLocalizedName("шт", "pcs", lang),
		}
	} else {
		res.QuantityValue = v.QuantityValue
		if v.UnitID != nil && v.Unit.ID != 0 {
			res.Unit = &UnitResponse{
				ID:        v.Unit.ID,
				Name:      getLocalized(v.Unit.Name, lang),
				ShortName: getLocalized(v.Unit.ShortName, lang),
			}
		}
	}

	if currentLangTranslation != nil {
		res.Name = currentLangTranslation.Name
	}
	// Власна назва варіації має пріоритет над назвою товару.
	if vn := getLocalized(v.Name, lang); vn != "" {
		res.Name = vn
	}

	// Шукаємо primary image
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

	// Бейджі: об'єднуємо бейджі товару та варіації + computed badges, без дублікатів
	res.Badges = mergeAndDeduplicateBadges(v.Product.Badges, v.Badges, v.ComputedBadges, lang)

	return res
}

// --- Admin DTOs ---

type BundleItemInput struct {
	VariationID uuid.UUID `json:"variation_id" binding:"required"`
	Quantity    int       `json:"quantity" binding:"required,min=1"`
}

type CreateProductRequest struct {
	BrandID           uuid.UUID         `json:"brand_id" binding:"required"`
	CategoryID        uuid.UUID         `json:"category_id" binding:"required"`
	IsActive          *bool             `json:"is_active" binding:"required"`
	IsBundle          bool              `json:"is_bundle"`
	PriceStrategy     string            `json:"price_strategy"`
	NameUk            string            `json:"name_uk" binding:"required"`
	NameEn            string            `json:"name_en"` // optional, fallback to Uk in service
	DescriptionUk     string            `json:"description_uk"`
	DescriptionEn     string            `json:"description_en"`
	UsageUk           string            `json:"usage_instructions_uk"`
	UsageEn           string            `json:"usage_instructions_en"`
	MetaTitleUk       string            `json:"meta_title_uk"`
	MetaTitleEn       string            `json:"meta_title_en"`
	MetaDescriptionUk string            `json:"meta_description_uk"`
	MetaDescriptionEn string            `json:"meta_description_en"`
	MetaKeywordsUk    string            `json:"meta_keywords_uk"`
	MetaKeywordsEn    string            `json:"meta_keywords_en"`
	IsRecommended     *bool             `json:"is_recommended"`
	Variations        []VariationInput  `json:"variations" binding:"required,min=1"`
	AttributeValues   []AttributeInput  `json:"attribute_values"`
	ImagesMeta        []ImageMeta       `json:"images_meta"`
	BadgeIDs          []int             `json:"badge_ids"`
	BundleItems       []BundleItemInput `json:"bundle_items"`
}

type UpdateProductRequest struct {
	BrandID           *uuid.UUID         `json:"brand_id"`
	CategoryID        *uuid.UUID         `json:"category_id"`
	IsActive          *bool              `json:"is_active"`
	IsBundle          *bool              `json:"is_bundle"`
	PriceStrategy     *string            `json:"price_strategy"`
	NameUk            *string            `json:"name_uk"`
	NameEn            *string            `json:"name_en"`
	DescriptionUk     *string            `json:"description_uk"`
	DescriptionEn     *string            `json:"description_en"`
	UsageUk           *string            `json:"usage_instructions_uk"`
	UsageEn           *string            `json:"usage_instructions_en"`
	MetaTitleUk       *string            `json:"meta_title_uk"`
	MetaTitleEn       *string            `json:"meta_title_en"`
	MetaDescriptionUk *string            `json:"meta_description_uk"`
	MetaDescriptionEn *string            `json:"meta_description_en"`
	MetaKeywordsUk    *string            `json:"meta_keywords_uk"`
	MetaKeywordsEn    *string            `json:"meta_keywords_en"`
	IsRecommended     *bool              `json:"is_recommended"`
	Variations        *[]VariationInput  `json:"variations"`       // nil = не чіпати; [] = видалити всі; [...] = full sync
	AttributeValues   *[]AttributeInput  `json:"attribute_values"` // nil = не чіпати; [] = видалити всі; [...] = full sync
	ImagesToDelete    []uuid.UUID        `json:"images_to_delete"` // IDs of existing images to remove
	ImagesMeta        []ImageMeta        `json:"images_meta"`
	BadgeIDs          *[]int             `json:"badge_ids"`    // nil = не чіпати; [] = видалити всі; [...] = full sync
	BundleItems       *[]BundleItemInput `json:"bundle_items"` // nil = не чіпати; [] = видалити всі; [...] = full sync
}

type VariationInput struct {
	ID              *uuid.UUID       `json:"id,omitempty"` // For updates, nil means new variation
	NameUk          *string          `json:"name_uk"`      // власна назва варіації (опційно); порожня → назва товару
	NameEn          *string          `json:"name_en"`
	SKU             *string          `json:"sku"`
	Barcode         *string          `json:"barcode"`
	Price           *int             `json:"price"`
	OldPrice        *int             `json:"old_price"`
	QuantityValue   *float64         `json:"quantity_value"`
	Weight          *float64         `json:"weight"`
	UnitID          *int             `json:"unit_id"`
	IsActive        *bool            `json:"is_active"`
	AttributeValues []AttributeInput `json:"attribute_values"`
	BadgeIDs        []int            `json:"badge_ids"`
}

type AttributeInput struct {
	AttributeID   int      `json:"attribute_id" binding:"required"`
	ValueCode     string   `json:"value_code"` // дискретний код для фільтрації; якщо порожній — виводиться зі value_string
	ValueStringUk string   `json:"value_string_uk"`
	ValueStringEn string   `json:"value_string_en"`
	ValueNumeric  *float64 `json:"value_numeric"`
	UnitID        *int     `json:"unit_id"`
}

type ImageMeta struct {
	Key         string     `json:"key"` // Key in multipart form (e.g. "image_1")
	IsPrimary   bool       `json:"is_primary"`
	IsHover     bool       `json:"is_hover"`
	SortOrder   int        `json:"sort_order"`
	VariationID *uuid.UUID `json:"variation_id"`
	AltTextUk   string     `json:"alt_text_uk"`
	AltTextEn   string     `json:"alt_text_en"`
}

// --- Image Management DTOs ---

type UpdateImageRequest struct {
	IsPrimary      *bool      `json:"is_primary"`
	IsHover        *bool      `json:"is_hover"`
	SortOrder      *int       `json:"sort_order"`
	VariationIDSet *bool      `json:"variation_id_set"` // true = застосувати variation_id
	VariationID    *uuid.UUID `json:"variation_id"`     // nil = загальне фото; uuid = фото варіації
	AltTextUk      *string    `json:"alt_text_uk"`
	AltTextEn      *string    `json:"alt_text_en"`
}

type ReorderImagesRequest struct {
	IDs []uuid.UUID `json:"ids" binding:"required"`
}

type CreateBrandRequest struct {
	Slug string `json:"slug"`
	Name string `json:"name" binding:"required,max=100"`
}

type UpdateBrandRequest struct {
	Slug *string `json:"slug"`
	Name *string `json:"name" binding:"omitempty,max=100"`
}

type AttributeResponse struct {
	ID                int           `json:"id"`
	Code              string        `json:"code"`
	Name              string        `json:"name"`
	SortOrder         int           `json:"sort_order"`
	IsFilterable      bool          `json:"is_filterable"`
	IsVariantSpecific bool          `json:"is_variant_specific"`
	Unit              *UnitResponse `json:"unit,omitempty"`
}

type CreateAttributeRequest struct {
	Code              string `json:"code" binding:"required,max=50"`
	NameUk            string `json:"name_uk" binding:"required,max=100"`
	NameEn            string `json:"name_en"`
	SortOrder         int    `json:"sort_order"`
	IsFilterable      bool   `json:"is_filterable"`
	IsVariantSpecific bool   `json:"is_variant_specific"`
	UnitID            *int   `json:"unit_id"`
}

type UpdateAttributeRequest struct {
	Code              *string `json:"code" binding:"omitempty,max=50"`
	NameUk            *string `json:"name_uk" binding:"omitempty,max=100"`
	NameEn            *string `json:"name_en"`
	SortOrder         *int    `json:"sort_order"`
	IsFilterable      *bool   `json:"is_filterable"`
	IsVariantSpecific *bool   `json:"is_variant_specific"`
	UnitID            *int    `json:"unit_id"`
}

func mapAttributeToResponse(attr domain.Attribute) AttributeResponse {
	name := ""
	if len(attr.Translations) > 0 {
		name = attr.Translations[0].Name
	}
	var unit *UnitResponse
	if attr.Unit != nil {
		unit = &UnitResponse{
			ID:        attr.Unit.ID,
			Name:      attr.Unit.Name["uk"], // Or current lang
			ShortName: attr.Unit.ShortName["uk"],
		}
	}
	return AttributeResponse{
		ID:                attr.ID,
		Code:              attr.Code,
		Name:              name,
		SortOrder:         attr.SortOrder,
		IsFilterable:      attr.IsFilterable,
		IsVariantSpecific: attr.IsVariantSpecific,
		Unit:              unit,
	}
}

func mapAttributeListToResponse(attrs []domain.Attribute) []AttributeResponse {
	res := make([]AttributeResponse, len(attrs))
	for i, a := range attrs {
		res[i] = mapAttributeToResponse(a)
	}
	return res
}

// mapProductBadgesToResponse конвертує ProductBadge slice у BadgeResponse slice
func mapProductBadgesToResponse(badges []domain.ProductBadge, lang string) []BadgeResponse {
	res := make([]BadgeResponse, 0, len(badges))
	for _, pb := range badges {
		res = append(res, BadgeResponse{
			ID:       pb.Badge.ID,
			Name:     getLocalized(pb.Badge.Name, lang),
			ColorHex: pb.Badge.ColorHex,
		})
	}
	return res
}

// mergeAndDeduplicateBadges об'єднує бейджі товару, варіації та обчислені бейджі, видаляючи дублікати за ID.
func mergeAndDeduplicateBadges(productBadges, variationBadges []domain.ProductBadge, computedBadges []domain.Badge, lang string) []BadgeResponse {
	seen := make(map[int]bool)
	res := make([]BadgeResponse, 0)

	// 1. Спочатку обчислені (автоматичні) бейджі — вони мають пріоритет у назвах
	for _, b := range computedBadges {
		if !seen[b.ID] {
			seen[b.ID] = true
			res = append(res, BadgeResponse{
				ID:       b.ID,
				Name:     getLocalized(b.Name, lang),
				ColorHex: b.ColorHex,
			})
		}
	}

	// 2. Потім бейджі товару
	for _, pb := range productBadges {
		if !seen[pb.Badge.ID] {
			seen[pb.Badge.ID] = true
			res = append(res, BadgeResponse{
				ID:       pb.Badge.ID,
				Name:     getLocalized(pb.Badge.Name, lang),
				ColorHex: pb.Badge.ColorHex,
			})
		}
	}

	// 3. Потім бейджі варіації
	for _, pb := range variationBadges {
		if !seen[pb.Badge.ID] {
			seen[pb.Badge.ID] = true
			res = append(res, BadgeResponse{
				ID:       pb.Badge.ID,
				Name:     getLocalized(pb.Badge.Name, lang),
				ColorHex: pb.Badge.ColorHex,
			})
		}
	}

	return res
}

// --- Admin Badge DTOs ---

type CreateBadgeRequest struct {
	NameUk    string `json:"name_uk" binding:"required,max=100"`
	NameEn    string `json:"name_en"`
	ColorHex  string `json:"color_hex" binding:"required,max=10"`
	SortOrder int    `json:"sort_order"`
}

type UpdateBadgeRequest struct {
	NameUk    *string `json:"name_uk" binding:"omitempty,max=100"`
	NameEn    *string `json:"name_en"`
	ColorHex  *string `json:"color_hex" binding:"omitempty,max=10"`
	SortOrder *int    `json:"sort_order"`
}

type AdminBadgeResponse struct {
	ID        int               `json:"id"`
	Name      map[string]string `json:"name"`
	ColorHex  string            `json:"color_hex"`
	SortOrder int               `json:"sort_order"`
	CreatedAt time.Time         `json:"created_at"`
}

func mapBadgeToAdminResponse(b domain.Badge) AdminBadgeResponse {
	return AdminBadgeResponse{
		ID:        b.ID,
		Name:      b.Name,
		ColorHex:  b.ColorHex,
		SortOrder: b.SortOrder,
		CreatedAt: b.CreatedAt,
	}
}

func mapBadgeListToAdminResponse(badges []domain.Badge) []AdminBadgeResponse {
	res := make([]AdminBadgeResponse, len(badges))
	for i, b := range badges {
		res[i] = mapBadgeToAdminResponse(b)
	}
	return res
}
