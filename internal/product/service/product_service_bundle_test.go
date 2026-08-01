package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
)

func TestProductService_CreateProduct_Bundle_Success(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	bundleProductID := uuid.New()
	compVarID1 := uuid.New()
	compVarID2 := uuid.New()

	product := &domain.Product{
		ID:            bundleProductID,

		IsBundle:      true,
		PriceStrategy: domain.PriceStrategyManual,
		Translations: []domain.ProductTranslation{
			{ProductID: bundleProductID, LanguageCode: "uk", Name: "Набір товарів"},
		},
		Variations: []domain.ProductVariation{
			{ID: uuid.New(), SKU: "BUNDLE-01", Price: 50000},
		},
		BundleItems: []domain.ProductBundleItem{
			{BundleID: bundleProductID, VariationID: compVarID1, Quantity: 2},
			{BundleID: bundleProductID, VariationID: compVarID2, Quantity: 1},
		},
	}

	repo.On("AreVariationsNonBundle", mock.Anything, []uuid.UUID{compVarID1, compVarID2}).Return(true, nil)
	mockCompVars := []domain.ProductVariation{
		{
			ID:     compVarID1,
			Price:  15000,
			Weight: 1.0,
			Product: domain.Product{
				Translations: []domain.ProductTranslation{
					{LanguageCode: "uk", Name: "Шампунь"},
					{LanguageCode: "en", Name: "Shampoo"},
				},
			},
		},
		{
			ID:     compVarID2,
			Price:  20000,
			Weight: 0.5,
			Product: domain.Product{
				Translations: []domain.ProductTranslation{
					{LanguageCode: "uk", Name: "Мило"},
					{LanguageCode: "en", Name: "Soap"},
				},
			},
		},
	}
	repo.On("FindVariationsByIDs", mock.Anything, []uuid.UUID{compVarID1, compVarID2}).Return(mockCompVars, nil)
	
	mockCountAttr := &domain.Attribute{
		ID:     10,
		Code:   "bundle_items_count",
		UnitID: func() *int { id := 5; return &id }(),
	}
	mockItemsAttr := &domain.Attribute{
		ID:   20,
		Code: "bundle_items",
	}
	repo.On("FindAttributeByCode", mock.Anything, "bundle_items_count").Return(mockCountAttr, nil)
	repo.On("FindAttributeByCode", mock.Anything, "bundle_items").Return(mockItemsAttr, nil)
	repo.On("SlugExists", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(false, nil)
	repo.On("Create", mock.Anything, product).Return(nil)

	err := svc.CreateProduct(context.Background(), product, nil)
	require.NoError(t, err)
	assert.Equal(t, 2.5, product.Variations[0].Weight) // сума ваг компонентів: 1.0*2 + 0.5*1 = 2.5
	repo.AssertExpectations(t)
}

func TestProductService_CreateProduct_Bundle_NoComponents(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	bundleProductID := uuid.New()

	product := &domain.Product{
		ID:            bundleProductID,

		IsBundle:      true,
		PriceStrategy: domain.PriceStrategyManual,
		Translations: []domain.ProductTranslation{
			{ProductID: bundleProductID, LanguageCode: "uk", Name: "Набір товарів"},
		},
		Variations: []domain.ProductVariation{
			{ID: uuid.New(), SKU: "BUNDLE-01", Price: 50000},
		},
		BundleItems: []domain.ProductBundleItem{},
	}

	repo.On("SlugExists", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(false, nil)
	err := svc.CreateProduct(context.Background(), product, nil)
	assert.ErrorIs(t, err, domain.ErrBundleNoComponents)
}

func TestProductService_CreateProduct_Bundle_NestedBundle(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	bundleProductID := uuid.New()
	compVarID := uuid.New()

	product := &domain.Product{
		ID:            bundleProductID,

		IsBundle:      true,
		PriceStrategy: domain.PriceStrategyManual,
		Translations: []domain.ProductTranslation{
			{ProductID: bundleProductID, LanguageCode: "uk", Name: "Набір товарів"},
		},
		Variations: []domain.ProductVariation{
			{ID: uuid.New(), SKU: "BUNDLE-01", Price: 50000},
		},
		BundleItems: []domain.ProductBundleItem{
			{BundleID: bundleProductID, VariationID: compVarID, Quantity: 1},
		},
	}

	repo.On("AreVariationsNonBundle", mock.Anything, []uuid.UUID{compVarID}).Return(false, nil)

	repo.On("SlugExists", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(false, nil)
	err := svc.CreateProduct(context.Background(), product, nil)
	assert.ErrorIs(t, err, domain.ErrNestedBundle)
}

func TestProductService_UpdateProduct_Bundle_NestedBundle(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	bundleProductID := uuid.New()
	compVarID := uuid.New()

	existing := &domain.Product{
		ID:            bundleProductID,
		IsBundle:      true,
		PriceStrategy: domain.PriceStrategyManual,
	}

	updated := &domain.Product{
		ID:            bundleProductID,
		IsBundle:      true,
		PriceStrategy: domain.PriceStrategyManual,
		BundleItems: []domain.ProductBundleItem{
			{BundleID: bundleProductID, VariationID: compVarID, Quantity: 1},
		},
	}

	repo.On("FindByID", mock.Anything, bundleProductID, "uk").Return(existing, nil)
	repo.On("AreVariationsNonBundle", mock.Anything, []uuid.UUID{compVarID}).Return(false, nil)

	err := svc.UpdateProduct(context.Background(), updated, nil, nil, nil, nil, &updated.IsBundle, false)
	assert.ErrorIs(t, err, domain.ErrNestedBundle)
}

func TestProductService_GetByID_BundlePricing(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	bundleProductID := uuid.New()
	compVarID1 := uuid.New()
	compVarID2 := uuid.New()

	t.Run("Dynamic pricing - sums component prices", func(t *testing.T) {
		product := &domain.Product{
			ID:            bundleProductID,
			IsBundle:      true,
			PriceStrategy: domain.PriceStrategyDynamic,
			Variations: []domain.ProductVariation{
				{ID: uuid.New(), Price: 0},
			},
			BundleItems: []domain.ProductBundleItem{
				{
					BundleID:    bundleProductID,
					VariationID: compVarID1,
					Quantity:    2,
					Variation: domain.ProductVariation{
						Price: 15000,
					},
				},
				{
					BundleID:    bundleProductID,
					VariationID: compVarID2,
					Quantity:    1,
					Variation: domain.ProductVariation{
						Price: 20000,
					},
				},
			},
		}

		repo.On("FindByID", mock.Anything, bundleProductID, "uk").Return(product, nil).Once()

		res, err := svc.GetByID(context.Background(), bundleProductID, "uk")
		require.NoError(t, err)
		assert.Equal(t, 50000, res.Variations[0].Price) // 15000 * 2 + 20000 * 1
	})

	t.Run("Manual pricing - sets OldPrice to sum if nil and components > price", func(t *testing.T) {
		product := &domain.Product{
			ID:            bundleProductID,
			IsBundle:      true,
			PriceStrategy: domain.PriceStrategyManual,
			Variations: []domain.ProductVariation{
				{ID: uuid.New(), Price: 40000, OldPrice: nil},
			},
			BundleItems: []domain.ProductBundleItem{
				{
					BundleID:    bundleProductID,
					VariationID: compVarID1,
					Quantity:    2,
					Variation: domain.ProductVariation{
						Price: 15000,
					},
				},
				{
					BundleID:    bundleProductID,
					VariationID: compVarID2,
					Quantity:    1,
					Variation: domain.ProductVariation{
						Price: 20000,
					},
				},
			},
		}

		repo.On("FindByID", mock.Anything, bundleProductID, "uk").Return(product, nil).Once()

		res, err := svc.GetByID(context.Background(), bundleProductID, "uk")
		require.NoError(t, err)
		require.NotNil(t, res.Variations[0].OldPrice)
		assert.Equal(t, 50000, *res.Variations[0].OldPrice)
		assert.Equal(t, 40000, res.Variations[0].Price)
	})
}

func TestProductService_UpdateProduct_SlugRegeneration(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	productID := uuid.New()

	existing := &domain.Product{
		ID:   productID,

		Translations: []domain.ProductTranslation{
			{LanguageCode: "uk", Name: "Стара назва"},
		},
	}

	updated := &domain.Product{
		ID: productID,
		Translations: []domain.ProductTranslation{
			{LanguageCode: "uk", Name: "Нова назва"},
		},
	}

	repo.On("FindByID", mock.Anything, productID, "uk").Return(existing, nil).Once()
	repo.On("Exists", mock.Anything, productID).Return(false, nil)
	repo.On("SlugExists", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(false, nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(p *domain.Product) bool {
		return p.Translations[0].Slug == "nova-nazva"
	})).Return(nil).Once()

	err := svc.UpdateProduct(context.Background(), updated, nil, nil, nil, nil, nil, false)
	assert.NoError(t, err)
}
