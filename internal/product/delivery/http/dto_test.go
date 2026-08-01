package http

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
)

func TestMergeAndDeduplicateBadges(t *testing.T) {
	lang := "uk"

	computedBadges := []domain.Badge{
		{ID: 1, Name: domain.LocalizedMap{"uk": "Новинка"}, ColorHex: "#3B82F6"},
		{ID: 3, Name: domain.LocalizedMap{"uk": "Знижка 20%"}, ColorHex: "#EF4444"},
	}

	productBadges := []domain.ProductBadge{
		{BadgeID: 1, Badge: domain.Badge{ID: 1, Name: domain.LocalizedMap{"uk": "Ручна Новинка"}, ColorHex: "#000000"}},
		{BadgeID: 2, Badge: domain.Badge{ID: 2, Name: domain.LocalizedMap{"uk": "Хіт"}, ColorHex: "#F59E0B"}},
	}

	variationBadges := []domain.ProductBadge{
		{BadgeID: 3, Badge: domain.Badge{ID: 3, Name: domain.LocalizedMap{"uk": "Ручна Знижка"}, ColorHex: "#FFFFFF"}},
		{BadgeID: 4, Badge: domain.Badge{ID: 4, Name: domain.LocalizedMap{"uk": "Лімітовано"}, ColorHex: "#888888"}},
	}

	res := mergeAndDeduplicateBadges(productBadges, variationBadges, computedBadges, lang)

	assert.Len(t, res, 4, "Should deduplicate to exactly 4 unique badges")

	var ids []int
	for _, b := range res {
		ids = append(ids, b.ID)
	}

	assert.Contains(t, ids, 1)
	assert.Contains(t, ids, 2)
	assert.Contains(t, ids, 3)
	assert.Contains(t, ids, 4)

	// Check priority overrides (Computed should override manual)
	var badge1, badge3 BadgeResponse
	for _, b := range res {
		if b.ID == 1 {
			badge1 = b
		}
		if b.ID == 3 {
			badge3 = b
		}
	}

	assert.Equal(t, "Новинка", badge1.Name, "Computed badge should override product-level manual badge")
	assert.Equal(t, "#3B82F6", badge1.ColorHex, "Should retain computed badge color")

	assert.Equal(t, "Знижка 20%", badge3.Name, "Computed badge should override variation-level manual badge")
}

func TestMergeAndDeduplicateBadges_Empty(t *testing.T) {
	res := mergeAndDeduplicateBadges(nil, nil, nil, "uk")
	assert.Empty(t, res)
}

func TestMapProductBadgesToResponse(t *testing.T) {
	badges := []domain.ProductBadge{
		{Badge: domain.Badge{ID: 1, Name: domain.LocalizedMap{"uk": "Бейдж"}, ColorHex: "#000"}},
	}
	res := mapProductBadgesToResponse(badges, "uk")
	assert.Len(t, res, 1)
	assert.Equal(t, 1, res[0].ID)
	assert.Equal(t, "Бейдж", res[0].Name)
}

func TestMapProductToResponse_BundleItemsCharacteristics(t *testing.T) {
	lang := "uk"
	bundleID := uuid.New()
	variationID := uuid.New()
	productID := uuid.New()

	p := domain.Product{
		ID:            bundleID,

		IsBundle:      true,
		PriceStrategy: "manual",
		Brand: domain.Brand{
			Name: "Bundle Brand",
		},
		BundleItems: []domain.ProductBundleItem{
			{
				BundleID:    bundleID,
				VariationID: variationID,
				Quantity:    2,
				Variation: domain.ProductVariation{
					ID:            variationID,
					ProductID:     productID,
					SKU:           "COMP-SKU",
					Barcode:       "1234567890123",
					Price:         500,
					QuantityValue: 50,
					Weight:        1.5,
					UnitID:        ptr(3),
					Unit: domain.Unit{
						ID:        3,
						Name:      domain.LocalizedMap{"uk": "Штуки", "en": "Pcs"},
						ShortName: domain.LocalizedMap{"uk": "шт", "en": "pcs"},
					},
					Product: domain.Product{
						ID:   productID,

						Brand: domain.Brand{
							Name: "Component Brand",
						},
						Translations: []domain.ProductTranslation{
							{
								LanguageCode: "uk",
								Name:         "Component Name",
								Slug:         "component-product",
							},
						},
					},
				},
			},
		},
	}

	resp := mapProductToResponse(p, lang, "", nil)

	assert.True(t, resp.IsBundle)
	require.Len(t, resp.BundleItems, 1)

	item := resp.BundleItems[0]
	assert.Equal(t, "Component Name", item.ProductName)
	assert.Equal(t, "component-product", item.ProductSlug)
	assert.Equal(t, 2, item.Quantity)
	assert.Equal(t, 50.0, item.QuantityValue)

	// Check characteristics synthesis
	characteristics := item.Characteristics
	// Expected synthesized: Brand (sort 10), Quantity (sort 40), Weight (sort 50), EAN (sort 90)
	var brandChar, qtyChar, weightChar, eanChar *CharacteristicResponse
	for i := range characteristics {
		c := &characteristics[i]
		switch c.Code {
		case "brand":
			brandChar = c
		case "quantity":
			qtyChar = c
		case "weight":
			weightChar = c
		case "ean":
			eanChar = c
		}
	}

	require.NotNil(t, brandChar, "Brand characteristic should be synthesized")
	assert.Equal(t, "Component Brand", *brandChar.ValueString)
	assert.Equal(t, 10, brandChar.SortOrder)

	require.NotNil(t, qtyChar, "Quantity characteristic should be synthesized")
	assert.Nil(t, qtyChar.ValueString)
	require.NotNil(t, qtyChar.ValueNumeric)
	assert.Equal(t, 50.0, *qtyChar.ValueNumeric)
	require.NotNil(t, qtyChar.Unit)
	assert.Equal(t, 3, qtyChar.Unit.ID)
	assert.Equal(t, "шт", qtyChar.Unit.ShortName)
	assert.Equal(t, 40, qtyChar.SortOrder)

	require.NotNil(t, weightChar, "Weight characteristic should be synthesized")
	assert.Nil(t, weightChar.ValueString)
	require.NotNil(t, weightChar.ValueNumeric)
	assert.Equal(t, 1.5, *weightChar.ValueNumeric)
	require.NotNil(t, weightChar.Unit)
	assert.Equal(t, 4, weightChar.Unit.ID)
	assert.Equal(t, "кг", weightChar.Unit.ShortName)
	assert.Equal(t, 50, weightChar.SortOrder)

	require.NotNil(t, eanChar, "EAN characteristic should be synthesized")
	assert.Equal(t, "1234567890123", *eanChar.ValueString)
	assert.Equal(t, 90, eanChar.SortOrder)

	// Verify they are sorted by SortOrder
	for i := 0; i < len(characteristics)-1; i++ {
		assert.True(t, characteristics[i].SortOrder <= characteristics[i+1].SortOrder, "Characteristics must be sorted by SortOrder ASC")
	}
}

func ptr[T any](v T) *T {
	return &v
}

func TestMapProductToResponse_VariationsSortingAndCharacteristics(t *testing.T) {
	lang := "uk"
	productID := uuid.New()

	p := domain.Product{
		ID:       productID,

		IsBundle: false,
		Brand: domain.Brand{
			Name: "Test Brand",
		},
		AttributeValues: []domain.AttributeValue{
			{
				ID:          uuid.New(),
				ProductID:   &productID,
				AttributeID: 2,
				Attribute: domain.Attribute{
					ID:                 2,
					Code:               "type",
					IsVariantSpecific:  false,
					Translations:       []domain.AttributeTranslation{{LanguageCode: "uk", Name: "Тип товару"}},
					SortOrder:          20,
				},
				ValueString: domain.LocalizedMap{"uk": "Таблетки"},
			},
		},
		Variations: []domain.ProductVariation{
			{
				ID:            uuid.New(),
				ProductID:     productID,
				SKU:           "VAR-60",
				Price:         40000,
				QuantityValue: 60,
				Weight:        0.5,
				UnitID:        ptr(3),
				Unit: domain.Unit{
					ID:        3,
					Name:      domain.LocalizedMap{"uk": "Штуки", "en": "Pcs"},
					ShortName: domain.LocalizedMap{"uk": "шт", "en": "pcs"},
				},
			},
			{
				ID:            uuid.New(),
				ProductID:     productID,
				SKU:           "VAR-30",
				Price:         25000,
				QuantityValue: 30,
				Weight:        0.5,
				UnitID:        ptr(3),
				Unit: domain.Unit{
					ID:        3,
					Name:      domain.LocalizedMap{"uk": "Штуки", "en": "Pcs"},
					ShortName: domain.LocalizedMap{"uk": "шт", "en": "pcs"},
				},
			},
		},
	}

	resp := mapProductToResponse(p, lang, "", nil)

	// Verify Variations are sorted by QuantityValue ASC
	require.Len(t, resp.Variations, 2)
	assert.Equal(t, "VAR-30", resp.Variations[0].SKU)
	assert.Equal(t, "VAR-60", resp.Variations[1].SKU)

	// Verify combined characteristics in each variation
	chars := resp.Variations[0].Characteristics
	require.Len(t, chars, 4)
	assert.Equal(t, "brand", chars[0].Code)
	assert.Equal(t, "type", chars[1].Code)
	assert.Equal(t, "quantity", chars[2].Code)
	assert.Equal(t, "weight", chars[3].Code)
}

func TestMapProductToResponse_BundleCharacteristics(t *testing.T) {
	lang := "uk"
	bundleID := uuid.New()
	variationID := uuid.New()
	productID := uuid.New()

	p := domain.Product{
		ID:            bundleID,

		IsBundle:      true,
		PriceStrategy: "manual",
		Brand: domain.Brand{
			Name: "Brand",
		},
		AttributeValues: []domain.AttributeValue{
			{
				AttributeID: 101,
				Attribute: domain.Attribute{
					ID:           101,
					Code:         "bundle_items_count",
					Translations: []domain.AttributeTranslation{{LanguageCode: "uk", Name: "Кількість товарів у наборі"}},
				},
				ValueNumeric: func() *float64 { f := 2.0; return &f }(),
				Unit: &domain.Unit{
					ID:        6,
					Name:      domain.LocalizedMap{"uk": "товари", "en": "products"},
					ShortName: domain.LocalizedMap{"uk": "шт", "en": "pcs"},
				},
			},
			{
				AttributeID: 102,
				Attribute: domain.Attribute{
					ID:           102,
					Code:         "bundle_items",
					Translations: []domain.AttributeTranslation{{LanguageCode: "uk", Name: "Товари у наборі"}},
				},
				ValueString: domain.LocalizedMap{
					"uk": "Шампунь (2 шт.)",
					"en": "Shampoo (2 pcs)",
				},
			},
		},
		Variations: []domain.ProductVariation{
			{
				ID:     uuid.New(),
				Weight: 3.5,
			},
		},
		BundleItems: []domain.ProductBundleItem{
			{
				BundleID:    bundleID,
				VariationID: variationID,
				Quantity:    2,
				Variation: domain.ProductVariation{
					ID:        variationID,
					ProductID: productID,
					Product: domain.Product{
						ID:   productID,

						Translations: []domain.ProductTranslation{
							{
								LanguageCode: "uk",
								Name:         "Шампунь",
							},
						},
					},
				},
			},
		},
	}

	resp := mapProductToResponse(p, lang, "", nil)

	// Verify common characteristics for bundle - should be exactly 3
	assert.Equal(t, 3, len(resp.Variations[0].Characteristics))

	var itemsCountChar, bundleItemsChar, weightChar *CharacteristicResponse
	for i := range resp.Variations[0].Characteristics {
		c := &resp.Variations[0].Characteristics[i]
		switch c.Code {
		case "bundle_items_count":
			itemsCountChar = c
		case "bundle_items":
			bundleItemsChar = c
		case "weight":
			weightChar = c
		}
	}

	require.NotNil(t, itemsCountChar)
	assert.Equal(t, 2.0, *itemsCountChar.ValueNumeric)
	assert.Equal(t, "шт", itemsCountChar.Unit.ShortName)

	require.NotNil(t, bundleItemsChar)
	assert.Equal(t, "Товари у наборі", bundleItemsChar.Name)
	assert.Equal(t, "Шампунь (2 шт.)", *bundleItemsChar.ValueString)

	require.NotNil(t, weightChar)
	assert.Equal(t, 3.5, *weightChar.ValueNumeric)
	assert.Equal(t, "кг", weightChar.Unit.ShortName)
}

func TestMapReviewToResponse_UserNames(t *testing.T) {
	review := domain.ProductReview{
		ID:            uuid.New(),
		ProductID:     uuid.New(),
		UserID:        uuid.New(),
		Rating:        5,
		Comment:       "Чудовий товар!",
		UserFirstName: "Іван",
		UserLastName:  "Іванов",
	}

	resp := mapReviewToResponse(review)

	assert.Equal(t, review.ID, resp.ID)
	assert.Equal(t, review.UserID, resp.UserID)
	assert.Equal(t, "Іван", resp.UserFirstName)
	assert.Equal(t, "Іванов", resp.UserLastName)
	assert.Equal(t, 5, *resp.Rating)
	assert.Equal(t, "Чудовий товар!", resp.Comment)
}

func TestMapVariationToBriefResponse(t *testing.T) {
	lang := "uk"

	// 1. Standard product variation mapping
	stdVar := domain.ProductVariation{
		ID:            uuid.New(),
		ProductID:     uuid.New(),
		SKU:           "STD-001",
		Price:         100,
		QuantityValue: 22.0,
		UnitID:        ptr(3),
		Unit: domain.Unit{
			ID:        3,
			Name:      domain.LocalizedMap{"uk": "Штуки"},
			ShortName: domain.LocalizedMap{"uk": "шт"},
		},
		Product: domain.Product{
			IsBundle: false,
			Brand: domain.Brand{
				Name: "Brand",
			},
		},
	}

	stdResp := mapVariationToBriefResponse(stdVar, lang)
	assert.False(t, stdResp.IsBundle)
	assert.Equal(t, 22.0, stdResp.QuantityValue)
	require.NotNil(t, stdResp.Unit)
	assert.Equal(t, "шт", stdResp.Unit.ShortName)

	// 2. Bundle product variation mapping
	bundleVar := domain.ProductVariation{
		ID:            uuid.New(),
		ProductID:     uuid.New(),
		SKU:           "BDL-001",
		Price:         500,
		QuantityValue: 22.0, // should be overwritten
		UnitID:        ptr(3), // should be overwritten
		Unit: domain.Unit{
			ID:        3,
			Name:      domain.LocalizedMap{"uk": "Штуки"},
			ShortName: domain.LocalizedMap{"uk": "шт"},
		},
		Product: domain.Product{
			IsBundle: true,
			Brand: domain.Brand{
				Name: "Brand",
			},
			BundleItems: []domain.ProductBundleItem{
				{Quantity: 2},
				{Quantity: 2},
			},
		},
	}

	bundleResp := mapVariationToBriefResponse(bundleVar, lang)
	assert.True(t, bundleResp.IsBundle)
	assert.Equal(t, 4.0, bundleResp.QuantityValue)
	require.NotNil(t, bundleResp.Unit)
	assert.Equal(t, 6, bundleResp.Unit.ID)
	assert.Equal(t, "шт", bundleResp.Unit.ShortName)
}

func TestMapProductToResponse_VariationImageSlots(t *testing.T) {
	lang := "uk"
	varID1 := uuid.New()
	varID2 := uuid.New()
	pID := uuid.New()

	baseProduct := domain.Product{
		ID: pID,
		Variations: []domain.ProductVariation{
			{ID: varID1},
			{ID: varID2},
		},
	}

	t.Run("1. Лише загальні фото", func(t *testing.T) {
		p := baseProduct
		p.Images = []domain.ProductImage{
			{ID: uuid.New(), VariationID: nil, IsPrimary: true, SortOrder: 1, ImageURL: "gen-prim"},
			{ID: uuid.New(), VariationID: nil, IsHover: true, SortOrder: 2, ImageURL: "gen-hov"},
			{ID: uuid.New(), VariationID: nil, IsPrimary: false, IsHover: false, SortOrder: 3, ImageURL: "gen-add"},
		}

		res := mapProductToResponse(p, lang, "", nil)
		require.Len(t, res.Variations, 2)
		
		v1 := res.Variations[0]
		assert.Len(t, v1.Images, 3)
		assert.Equal(t, "gen-prim", v1.Images[0].ImageURL)
		assert.True(t, v1.Images[0].IsPrimary)
		
		assert.Equal(t, "gen-hov", v1.Images[1].ImageURL)
		assert.True(t, v1.Images[1].IsHover)
		
		assert.Equal(t, "gen-add", v1.Images[2].ImageURL)
	})

	t.Run("2. Власне головне, але hover і додаткові — загальні", func(t *testing.T) {
		p := baseProduct
		p.Images = []domain.ProductImage{
			{ID: uuid.New(), VariationID: nil, IsPrimary: true, SortOrder: 1, ImageURL: "gen-prim"},
			{ID: uuid.New(), VariationID: nil, IsHover: true, SortOrder: 2, ImageURL: "gen-hov"},
			{ID: uuid.New(), VariationID: nil, IsPrimary: false, IsHover: false, SortOrder: 3, ImageURL: "gen-add"},
			// Власне головне для varID1
			{ID: uuid.New(), VariationID: &varID1, IsPrimary: true, SortOrder: 1, ImageURL: "own-prim"},
		}

		res := mapProductToResponse(p, lang, "", nil)
		v1 := res.Variations[0] // varID1
		
		assert.Len(t, v1.Images, 3)
		// 1. Власне головне
		assert.Equal(t, "own-prim", v1.Images[0].ImageURL)
		assert.True(t, v1.Images[0].IsPrimary)
		
		// 2. Загальний hover
		assert.Equal(t, "gen-hov", v1.Images[1].ImageURL)
		assert.True(t, v1.Images[1].IsHover)
		
		// 3. Загальні додаткові
		assert.Equal(t, "gen-add", v1.Images[2].ImageURL)
	})

	t.Run("3. Власне додаткове фото, а головне/hover — загальні", func(t *testing.T) {
		p := baseProduct
		p.Images = []domain.ProductImage{
			{ID: uuid.New(), VariationID: nil, IsPrimary: true, SortOrder: 1, ImageURL: "gen-prim"},
			{ID: uuid.New(), VariationID: nil, IsHover: true, SortOrder: 2, ImageURL: "gen-hov"},
			{ID: uuid.New(), VariationID: nil, IsPrimary: false, IsHover: false, SortOrder: 3, ImageURL: "gen-add1"},
			{ID: uuid.New(), VariationID: nil, IsPrimary: false, IsHover: false, SortOrder: 4, ImageURL: "gen-add2"},
			// Власне додаткове для varID1
			{ID: uuid.New(), VariationID: &varID1, IsPrimary: false, IsHover: false, SortOrder: 1, ImageURL: "own-add1"},
		}

		res := mapProductToResponse(p, lang, "", nil)
		v1 := res.Variations[0]
		
		assert.Len(t, v1.Images, 3, "Має бути primary(gen), hover(gen) та 1 own additional (gen additional відкидаються)")
		
		assert.Equal(t, "gen-prim", v1.Images[0].ImageURL)
		assert.True(t, v1.Images[0].IsPrimary)
		
		assert.Equal(t, "gen-hov", v1.Images[1].ImageURL)
		assert.True(t, v1.Images[1].IsHover)
		
		assert.Equal(t, "own-add1", v1.Images[2].ImageURL)
		assert.False(t, v1.Images[2].IsPrimary)
		assert.False(t, v1.Images[2].IsHover)
	})

	t.Run("4. Власні головне, hover та додаткові — показуються тільки вони", func(t *testing.T) {
		p := baseProduct
		p.Images = []domain.ProductImage{
			{ID: uuid.New(), VariationID: nil, IsPrimary: true, SortOrder: 1, ImageURL: "gen-prim"},
			{ID: uuid.New(), VariationID: nil, IsHover: true, SortOrder: 2, ImageURL: "gen-hov"},
			{ID: uuid.New(), VariationID: nil, IsPrimary: false, IsHover: false, SortOrder: 3, ImageURL: "gen-add"},
			// Власні для varID1
			{ID: uuid.New(), VariationID: &varID1, IsPrimary: true, SortOrder: 1, ImageURL: "own-prim"},
			{ID: uuid.New(), VariationID: &varID1, IsHover: true, SortOrder: 2, ImageURL: "own-hov"},
			{ID: uuid.New(), VariationID: &varID1, IsPrimary: false, IsHover: false, SortOrder: 3, ImageURL: "own-add"},
		}

		res := mapProductToResponse(p, lang, "", nil)
		v1 := res.Variations[0]
		
		assert.Len(t, v1.Images, 3)
		assert.Equal(t, "own-prim", v1.Images[0].ImageURL)
		assert.Equal(t, "own-hov", v1.Images[1].ImageURL)
		assert.Equal(t, "own-add", v1.Images[2].ImageURL)
	})
	
	t.Run("5. Одне і те ж фото є і primary, і hover", func(t *testing.T) {
		id := uuid.New()
		p := baseProduct
		p.Images = []domain.ProductImage{
			{ID: id, VariationID: nil, IsPrimary: true, IsHover: true, SortOrder: 1, ImageURL: "gen-prim-hov"},
			{ID: uuid.New(), VariationID: nil, IsPrimary: false, IsHover: false, SortOrder: 2, ImageURL: "gen-add"},
		}

		res := mapProductToResponse(p, lang, "", nil)
		v1 := res.Variations[0]
		
		assert.Len(t, v1.Images, 2)
		assert.Equal(t, "gen-prim-hov", v1.Images[0].ImageURL)
		assert.True(t, v1.Images[0].IsPrimary)
		assert.True(t, v1.Images[0].IsHover)
		
		assert.Equal(t, "gen-add", v1.Images[1].ImageURL)
	})
}
