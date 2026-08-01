package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	slugLib "github.com/gosimple/slug"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type productRepository struct {
	db *gorm.DB
	l  logger.Logger
}

// NewProductRepository створює новий інстанс репозиторію
func NewProductRepository(db *gorm.DB, l logger.Logger) domain.ProductRepository {
	return &productRepository{db: db, l: l}
}

func applyProductFilters(q *gorm.DB, filter domain.ProductFilter, ignore []string) *gorm.DB {
	isIgnored := func(name string) bool {
		for _, i := range ignore {
			if i == name {
				return true
			}
		}
		return false
	}

	if !isIgnored("brand") && (len(filter.BrandID) > 0 || len(filter.BrandSlugs) > 0) {
		var brandConds []string
		var args []interface{}

		if len(filter.BrandID) > 0 {
			brandConds = append(brandConds, "product.brand_id IN (?)")
			args = append(args, filter.BrandID)
		}
		if len(filter.BrandSlugs) > 0 {
			brandConds = append(brandConds, "product.brand_id IN (SELECT id FROM brand WHERE slug IN (?) AND deleted_at IS NULL)")
			args = append(args, filter.BrandSlugs)
		}

		brandCondStr := strings.Join(brandConds, " OR ")

		var allArgs []interface{}
		allArgs = append(allArgs, args...) // For direct brand
		allArgs = append(allArgs, args...) // For bundle items

		bundleCondStr := strings.ReplaceAll(brandCondStr, "product.brand_id", "p_comp.brand_id")

		q = q.Where(fmt.Sprintf(`(
			(%s)
			OR EXISTS (
				SELECT 1 FROM product_bundle_item pbi
				JOIN product_variation pv_comp ON pv_comp.id = pbi.variation_id
				JOIN product p_comp ON p_comp.id = pv_comp.product_id
				WHERE pbi.bundle_id = product.id
				  AND (%s)
			)
		)`, brandCondStr, bundleCondStr), allArgs...)
	}

	if !isIgnored("price") {
		if filter.MinPrice != nil {
			q = q.Where("product_variation.price >= ?", *filter.MinPrice)
		}
		if filter.MaxPrice != nil {
			q = q.Where("product_variation.price <= ?", *filter.MaxPrice)
		}
	}

	if !isIgnored("quantity") && len(filter.QuantityValues) > 0 {
		var conditions []string
		var args []interface{}
		var simpleQuantities []float64

		for _, qStr := range filter.QuantityValues {
			val, unitID, ok := parseQuantityFilterValue(qStr)
			if ok {
				conditions = append(conditions, "(product_variation.quantity_value = ? AND product_variation.unit_id = ?)")
				args = append(args, val, unitID)
			} else if val > 0 || qStr == "0" {
				simpleQuantities = append(simpleQuantities, val)
			}
		}

		if len(simpleQuantities) > 0 {
			conditions = append(conditions, "product_variation.quantity_value IN (?)")
			args = append(args, simpleQuantities)
		}

		if len(conditions) > 0 {
			conditionStr := strings.Join(conditions, " OR ")
			bundleConditionStr := strings.ReplaceAll(conditionStr, "product_variation.", "pv_comp.")

			var allArgs []interface{}
			allArgs = append(allArgs, args...)
			allArgs = append(allArgs, args...)

			q = q.Where(fmt.Sprintf(`(
				(%s)
				OR EXISTS (
					SELECT 1 FROM product_bundle_item pbi
					JOIN product_variation pv_comp ON pv_comp.id = pbi.variation_id
					WHERE pbi.bundle_id = product.id
					  AND (%s)
				)
			)`, conditionStr, bundleConditionStr), allArgs...)
		}
	}

	if !isIgnored("attributes") && len(filter.AttrValues) > 0 {
		for code, values := range filter.AttrValues {
			if len(values) == 0 {
				continue
			}
			if isIgnored("attr:" + code) {
				continue
			}

			lowerCode := strings.ToLower(code)
			lowerValues := make([]string, len(values))
			for i, v := range values {
				lowerValues[i] = strings.ToLower(v)
			}

			q = q.Where(`(EXISTS (
				SELECT 1 FROM attribute_value av
				JOIN attribute a ON a.id = av.attribute_id AND a.deleted_at IS NULL
				WHERE a.code = ?
				  AND (av.product_id = product_variation.product_id OR av.variation_id = product_variation.id)
				  AND av.value_code IN (?)
			) OR EXISTS (
				SELECT 1 FROM product_bundle_item pbi
				JOIN product_variation pv_comp ON pv_comp.id = pbi.variation_id
				JOIN attribute_value av ON (av.product_id = pv_comp.product_id OR av.variation_id = pv_comp.id)
				JOIN attribute a ON a.id = av.attribute_id AND a.deleted_at IS NULL
				WHERE pbi.bundle_id = product.id
				  AND a.code = ?
				  AND av.value_code IN (?)
			))`, lowerCode, lowerValues, lowerCode, lowerValues)
		}
	}

	if !isIgnored("search") && filter.SearchQuery != "" {
		q = q.Where(`(
			EXISTS (
				SELECT 1 FROM product_translation pt_search 
				WHERE pt_search.product_id = product_variation.product_id 
				  AND pt_search.name ILIKE ?
			) 
			OR product_variation.sku ILIKE ?
		)`, "%"+filter.SearchQuery+"%", "%"+filter.SearchQuery+"%")
	}

	return q
}

func (r *productRepository) FindAll(ctx context.Context, lang string, filter domain.ProductFilter, pgn pagination.Params) ([]domain.ProductVariation, int64, error) {
	var variations []domain.ProductVariation
	var totalItems int64

	// Початковий запит по варіаціях з JOIN на продукти
	query := r.db.WithContext(ctx).Model(&domain.ProductVariation{}).
		Joins("JOIN product ON product.id = product_variation.product_id AND product.deleted_at IS NULL").
		Where("product_variation.is_active = ? AND product.is_active = ?", true, true).
		Where(`(
			product.is_bundle = false
			OR NOT EXISTS (
				SELECT 1 FROM product_bundle_item pbi
				JOIN product_variation pv_comp ON pv_comp.id = pbi.variation_id
				JOIN product p_comp ON p_comp.id = pv_comp.product_id
				WHERE pbi.bundle_id = product.id
				  AND (pv_comp.is_active = false OR pv_comp.deleted_at IS NOT NULL OR p_comp.is_active = false OR p_comp.deleted_at IS NOT NULL)
			)
		)`)

	// Фільтрація
	if len(filter.CategoryID) > 0 {
		query = query.Where("product.category_id IN (?)", filter.CategoryID)
	}
	query = applyProductFilters(query, filter, nil)

	// Рахуємо загальну кількість варіацій
	if err := query.Count(&totalItems).Error; err != nil {
		r.l.Errorw("failed to count variations", "error", err)
		return nil, 0, err
	}

	// Сортування
	var orderExpr *clause.Expr
	orderBy := "product_variation.created_at DESC"
	sortByConf := strings.ToLower(pgn.SortBy)
	orderConf := strings.ToUpper(pgn.Order)
	if orderConf != "ASC" && orderConf != "DESC" {
		orderConf = "DESC"
	}

	if sortByConf == "price" {
		orderBy = fmt.Sprintf("product_variation.price %s", orderConf)
	} else if sortByConf == "name" {
		query = query.Joins("LEFT JOIN product_translation pt_sort ON pt_sort.product_id = product_variation.product_id AND pt_sort.language_code = ?", lang)
		orderBy = fmt.Sprintf("pt_sort.name %s", orderConf)
	} else if sortByConf == "rating" {
		orderBy = fmt.Sprintf("product.average_rating %s", orderConf)
	} else if filter.SearchQuery != "" {
		// Якщо сортування не вказане явно і є пошуковий запит, сортуємо за релевантністю.
		// Boost trigger, якщо ХОЧ ОДИН з перекладів назви починається з пошукового запиту.
		orderBy = ""
		orderExpr = &clause.Expr{SQL: `(
			EXISTS (
				SELECT 1 FROM product_translation pt_boost 
				WHERE pt_boost.product_id = product_variation.product_id 
				  AND pt_boost.name ILIKE ? || '%'
			)
		) DESC, product_variation.created_at DESC`, Vars: []interface{}{filter.SearchQuery}}
	}

	preloadQuery := query.
		Preload("Product", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Translations").
				Preload("Brand").
				Preload("Category.Translations").
				Preload("Images", func(db *gorm.DB) *gorm.DB {
					return db.Order("sort_order asc")
				}).
				Preload("Badges", func(db *gorm.DB) *gorm.DB {
					return db.Preload("Badge")
				}).
				Preload("BundleItems")
		}).
		Preload("Badges", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Badge")
		}).
		Preload("Unit")

	if orderBy != "" {
		preloadQuery = preloadQuery.Order(orderBy)
	}
	if orderExpr != nil {
		preloadQuery = preloadQuery.Order(*orderExpr)
	}

	err := preloadQuery.
		Limit(pgn.Limit).
		Offset(pgn.GetOffset()).
		Find(&variations).Error

	if err != nil {
		r.l.Errorw("failed to find variations", "error", err)
		return nil, 0, err
	}

	return variations, totalItems, nil
}

func (r *productRepository) FindByID(ctx context.Context, id uuid.UUID, lang string) (*domain.Product, error) {
	var product domain.Product

	err := r.db.WithContext(ctx).
		Preload("Translations").
		Preload("Brand").
		Preload("Category.Translations").
		Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order asc")
		}).
		Preload("AttributeValues.Attribute.Translations", "language_code = ?", lang).
		Preload("AttributeValues.Attribute.Unit").
		Preload("AttributeValues.Unit").
		Preload("Variations", "product_variation.is_active = ?", true).
		Preload("Variations.Unit").
		Preload("Variations.AttributeValues.Attribute.Translations", "language_code = ?", lang).
		Preload("Variations.AttributeValues.Attribute.Unit").
		Preload("Variations.AttributeValues.Unit").
		Preload("Variations.Badges", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Badge")
		}).
		Preload("Badges", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Badge")
		}).
		Where("product.id = ? AND product.is_active = ?", id, true).
		First(&product).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrProductNotFound
		}
		r.l.Errorw("failed to find product by id", "error", err, "id", id)
		return nil, err
	}

	// Preload bundle items with full component data
	if product.IsBundle {
		if err := r.preloadBundleItems(ctx, &product, lang); err != nil {
			r.l.Errorw("failed to preload bundle items", "error", err, "id", id)
			return nil, err
		}
		for _, item := range product.BundleItems {
			if item.Variation.ID == uuid.Nil || !item.Variation.IsActive || item.Variation.Product.ID == uuid.Nil || !item.Variation.Product.IsActive {
				return nil, domain.ErrProductNotFound
			}
		}
	}

	return &product, nil
}

func (r *productRepository) FindBySlug(ctx context.Context, slug string, lang string) (*domain.Product, error) {
	var product domain.Product

	err := r.db.WithContext(ctx).
		Preload("Translations").
		Preload("Brand").
		Preload("Category.Translations").
		Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order asc")
		}).
		Preload("AttributeValues.Attribute.Translations", "language_code = ?", lang).
		Preload("AttributeValues.Attribute.Unit").
		Preload("AttributeValues.Unit").
		Preload("Variations", "product_variation.is_active = ?", true).
		Preload("Variations.Unit").
		Preload("Variations.AttributeValues.Attribute.Translations", "language_code = ?", lang).
		Preload("Variations.AttributeValues.Attribute.Unit").
		Preload("Variations.AttributeValues.Unit").
		Preload("Variations.Badges", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Badge")
		}).
		Preload("Badges", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Badge")
		}).
		Joins("JOIN product_translation ON product_translation.product_id = product.id AND product_translation.language_code = ?", lang).
		Where("product_translation.slug = ? AND product.is_active = ?", slug, true).
		First(&product).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Try finding by variation slug
			var pv domain.ProductVariation
			errVar := r.db.WithContext(ctx).
				Select("product_id").
				Where("slug = ? AND is_active = ?", slug, true).
				First(&pv).Error
			if errVar == nil {
				// We found the variation! Load the parent product.
				err = r.db.WithContext(ctx).
					Preload("Translations").
					Preload("Brand").
					Preload("Category.Translations").
					Preload("Images", func(db *gorm.DB) *gorm.DB {
						return db.Order("sort_order asc")
					}).
					Preload("AttributeValues.Attribute.Translations", "language_code = ?", lang).
					Preload("AttributeValues.Attribute.Unit").
					Preload("AttributeValues.Unit").
					Preload("Variations", "product_variation.is_active = ?", true).
					Preload("Variations.Unit").
					Preload("Variations.AttributeValues.Attribute.Translations", "language_code = ?", lang).
					Preload("Variations.AttributeValues.Attribute.Unit").
					Preload("Variations.AttributeValues.Unit").
					Preload("Variations.Badges", func(db *gorm.DB) *gorm.DB {
						return db.Preload("Badge")
					}).
					Preload("Badges", func(db *gorm.DB) *gorm.DB {
						return db.Preload("Badge")
					}).
					Where("product.id = ? AND product.is_active = ?", pv.ProductID, true).
					First(&product).Error
			}
		}

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, domain.ErrSlugNotFound
			}
			r.l.Errorw("failed to find product by slug", "error", err, "slug", slug)
			return nil, err
		}
	}

	// Preload bundle items with full component data
	if product.IsBundle {
		if err := r.preloadBundleItems(ctx, &product, lang); err != nil {
			r.l.Errorw("failed to preload bundle items", "error", err, "slug", slug)
			return nil, err
		}
		for _, item := range product.BundleItems {
			if item.Variation.ID == uuid.Nil || !item.Variation.IsActive || item.Variation.Product.ID == uuid.Nil || !item.Variation.Product.IsActive {
				return nil, domain.ErrSlugNotFound
			}
		}
	}

	return &product, nil
}

func (r *productRepository) SlugExists(ctx context.Context, slug string, excludeID uuid.UUID) (bool, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&domain.ProductTranslation{}).Where("slug = ?", slug)
	if excludeID != uuid.Nil {
		query = query.Where("product_id != ?", excludeID)
	}
	err := query.Count(&count).Error
	if err != nil {
		r.l.Errorw("failed to check slug existence", "error", err, "slug", slug)
		return false, err
	}
	return count > 0, nil
}

func (r *productRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.Product{}).Where("product.id = ? AND product.is_active = ?", id, true).Count(&count).Error
	if err != nil {
		r.l.Errorw("failed to check product existence", "error", err, "id", id)
		return false, err
	}
	return count > 0, nil
}

func (r *productRepository) FindReviews(ctx context.Context, productID uuid.UUID, pgn pagination.Params) ([]domain.ProductReview, int64, error) {
	var reviews []domain.ProductReview
	var totalItems int64

	query := r.db.WithContext(ctx).Model(&domain.ProductReview{}).
		Select("product_review.*, u.first_name as user_first_name, u.last_name as user_last_name").
		Joins("LEFT JOIN \"user\" u ON u.id = product_review.user_id").
		Where("product_review.product_id = ? AND product_review.status = ?", productID, "approved")

	if err := query.Count(&totalItems).Error; err != nil {
		r.l.Errorw("failed to count product reviews", "error", err, "product_id", productID)
		return nil, 0, err
	}

	orderConf := "DESC"
	if strings.ToUpper(pgn.Order) == "ASC" {
		orderConf = "ASC"
	}

	err := query.Order(fmt.Sprintf("created_at %s", orderConf)).
		Limit(pgn.Limit).
		Offset(pgn.GetOffset()).
		Find(&reviews).Error

	if err != nil {
		r.l.Errorw("failed to find product reviews", "error", err, "product_id", productID)
		return nil, 0, err
	}

	return reviews, totalItems, nil
}

func (r *productRepository) FindPendingReviews(ctx context.Context, pgn pagination.Params) ([]domain.ProductReview, int64, error) {
	return r.findReviewsByStatus(ctx, "pending", pgn)
}

func (r *productRepository) FindRejectedReviews(ctx context.Context, pgn pagination.Params) ([]domain.ProductReview, int64, error) {
	return r.findReviewsByStatus(ctx, "rejected", pgn)
}

func (r *productRepository) findReviewsByStatus(ctx context.Context, status string, pgn pagination.Params) ([]domain.ProductReview, int64, error) {
	var reviews []domain.ProductReview
	var totalItems int64

	query := r.db.WithContext(ctx).Model(&domain.ProductReview{}).
		Select("product_review.*, u.first_name as user_first_name, u.last_name as user_last_name, pt.name as product_name, pt.slug as product_slug").
		Joins("LEFT JOIN \"user\" u ON u.id = product_review.user_id").
		Joins("LEFT JOIN product_translation pt ON pt.product_id = product_review.product_id AND pt.language_code = 'uk'").
		Where("product_review.status = ?", status)

	if err := query.Count(&totalItems).Error; err != nil {
		r.l.Errorw("failed to count reviews by status", "error", err, "status", status)
		return nil, 0, err
	}

	orderConf := "DESC"
	if strings.ToUpper(pgn.Order) == "ASC" {
		orderConf = "ASC"
	}

	err := query.Order(fmt.Sprintf("created_at %s", orderConf)).
		Limit(pgn.Limit).
		Offset(pgn.GetOffset()).
		Find(&reviews).Error

	if err != nil {
		r.l.Errorw("failed to find reviews by status", "error", err, "status", status)
		return nil, 0, err
	}

	return reviews, totalItems, nil
}

func (r *productRepository) CreateReview(ctx context.Context, review *domain.ProductReview) error {
	if err := r.db.WithContext(ctx).Create(review).Error; err != nil {
		r.l.Errorw("failed to create product review", "error", err, "product_id", review.ProductID)
		return err
	}
	return nil
}

func (r *productRepository) ApproveReview(ctx context.Context, reviewID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.ProductReview{}).Where("id = ?", reviewID).Update("status", "approved").Error; err != nil {
			r.l.Errorw("failed to approve review", "error", err, "review_id", reviewID)
			return err
		}

		var review domain.ProductReview
		if err := tx.Select("product_id").First(&review, "id = ?", reviewID).Error; err != nil {
			r.l.Errorw("failed to get product id for review", "error", err, "review_id", reviewID)
			return err
		}

		var stats struct {
			AvgRating float64
			Count     int
		}
		if err := tx.Model(&domain.ProductReview{}).
			Select("COALESCE(AVG(rating), 0) as avg_rating, COUNT(id) as count").
			Where("product_id = ? AND status = 'approved' AND parent_id IS NULL", review.ProductID).
			Scan(&stats).Error; err != nil {
			r.l.Errorw("failed to calculate review stats", "error", err, "product_id", review.ProductID)
			return err
		}

		if err := tx.Model(&domain.Product{}).
			Where("id = ?", review.ProductID).
			Updates(map[string]interface{}{
				"average_rating": stats.AvgRating,
				"reviews_count":  stats.Count,
			}).Error; err != nil {
			r.l.Errorw("failed to update product review stats", "error", err, "product_id", review.ProductID)
			return err
		}

		return nil
	})
}

func (r *productRepository) RejectReview(ctx context.Context, reviewID uuid.UUID, reason *string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"status": "rejected",
		}
		if reason != nil {
			updates["reject_reason"] = *reason
		}
		if err := tx.Model(&domain.ProductReview{}).Where("id = ?", reviewID).Updates(updates).Error; err != nil {
			r.l.Errorw("failed to reject review", "error", err, "review_id", reviewID)
			return err
		}

		var review domain.ProductReview
		if err := tx.Select("product_id").First(&review, "id = ?", reviewID).Error; err != nil {
			r.l.Errorw("failed to get product id for review", "error", err, "review_id", reviewID)
			return err
		}

		var stats struct {
			AvgRating float64
			Count     int
		}
		if err := tx.Model(&domain.ProductReview{}).
			Select("COALESCE(AVG(rating), 0) as avg_rating, COUNT(id) as count").
			Where("product_id = ? AND status = 'approved' AND parent_id IS NULL", review.ProductID).
			Scan(&stats).Error; err != nil {
			r.l.Errorw("failed to calculate review stats", "error", err, "product_id", review.ProductID)
			return err
		}

		if err := tx.Model(&domain.Product{}).
			Where("id = ?", review.ProductID).
			Updates(map[string]interface{}{
				"average_rating": stats.AvgRating,
				"reviews_count":  stats.Count,
			}).Error; err != nil {
			r.l.Errorw("failed to update product review stats", "error", err, "product_id", review.ProductID)
			return err
		}

		return nil
	})
}

func (r *productRepository) ReopenReview(ctx context.Context, reviewID uuid.UUID) error {
	result := r.db.WithContext(ctx).Model(&domain.ProductReview{}).
		Where("id = ? AND status = ?", reviewID, "rejected").
		Updates(map[string]interface{}{"status": "pending", "reject_reason": nil})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrReviewNotFound
	}
	return nil
}

// GetUnits повертає всі одиниці виміру (місткість/вага), відсортовані за id.
func (r *productRepository) GetUnits(ctx context.Context) ([]domain.Unit, error) {
	var units []domain.Unit
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&units).Error; err != nil {
		return nil, err
	}
	return units, nil
}

func (r *productRepository) GetFilters(ctx context.Context, filter domain.ProductFilter, lang string) (*domain.FilterDiscovery, error) {
	var res domain.FilterDiscovery

	// Базовий квері (тільки категорія та активність)
	baseQuery := r.db.WithContext(ctx).Model(&domain.ProductVariation{}).
		Joins("JOIN product ON product.id = product_variation.product_id AND product.deleted_at IS NULL").
		Where("product_variation.is_active = ? AND product.is_active = ?", true, true)

	if len(filter.CategoryID) > 0 {
		baseQuery = baseQuery.Where("product.category_id IN (?)", filter.CategoryID)
	}

	// Допоміжна функція для застосування інших фільтрів
	applyOtherFilters := func(q *gorm.DB, ignore []string) *gorm.DB {
		return applyProductFilters(q, filter, ignore)
	}

	// 1. Ціни
	var priceStats struct {
		MinPrice int
		MaxPrice int
	}
	if err := applyOtherFilters(baseQuery.Session(&gorm.Session{}), nil).
		Select("COALESCE(MIN(product_variation.price), 0) as min_price, COALESCE(MAX(product_variation.price), 0) as max_price").
		Scan(&priceStats).Error; err != nil {
		r.l.Errorw("failed to get price filters", "error", err)
		return nil, err
	}
	res.MinPrice = priceStats.MinPrice
	res.MaxPrice = priceStats.MaxPrice

	// 2. Бренди
	var brands []domain.BrandFilterOption
	if err := applyOtherFilters(baseQuery.Session(&gorm.Session{}), []string{"brand"}).
		Joins(`JOIN brand b ON (
			b.id = product.brand_id 
			OR EXISTS (
				SELECT 1 FROM product_bundle_item pbi
				JOIN product_variation pv_comp ON pv_comp.id = pbi.variation_id
				JOIN product p_comp ON p_comp.id = pv_comp.product_id
				WHERE pbi.bundle_id = product.id AND p_comp.brand_id = b.id
			)
		) AND b.deleted_at IS NULL`).
		Select("b.id, b.slug, b.name, COUNT(DISTINCT product_variation.id) as count").
		Group("b.id, b.slug, b.name").
		Order("b.name ASC").
		Scan(&brands).Error; err != nil {
		r.l.Errorw("failed to get brand filters", "error", err)
		return nil, err
	}
	res.Brands = brands

	// 3. Фасовки
	var quantities []domain.QuantityFilterOption
	if err := applyOtherFilters(baseQuery.Session(&gorm.Session{}), []string{"quantity"}).
		Joins(`CROSS JOIN LATERAL (
			SELECT product_variation.quantity_value AS val, product_variation.unit_id AS uid
			UNION
			SELECT pv_comp.quantity_value AS val, pv_comp.unit_id AS uid
			FROM product_bundle_item pbi
			JOIN product_variation pv_comp ON pv_comp.id = pbi.variation_id
			WHERE product.is_bundle = true AND pbi.bundle_id = product.id
		) q_val`).
		Joins("JOIN unit u ON u.id = q_val.uid AND u.deleted_at IS NULL").
		Select("q_val.val as value, u.id as unit_id, u.short_name->> ? as unit, COUNT(DISTINCT product_variation.id) as count", lang).
		Group("q_val.val, u.id, u.short_name").
		Order("value ASC").
		Scan(&quantities).Error; err != nil {
		r.l.Errorw("failed to get quantity filters", "error", err)
		return nil, err
	}
	res.Quantities = quantities

	// 4. Атрибути
	// Валідація мови для запобігання SQL-ін'єкцій у динамічних JSONB запитах
	if lang != "uk" && lang != "en" {
		lang = "uk"
	}

	var attrs []domain.Attribute
	if err := r.db.WithContext(ctx).Model(&domain.Attribute{}).
		Preload("Translations").
		Where("is_filterable = ?", true).
		Order("sort_order ASC").
		Find(&attrs).Error; err != nil {
		return nil, err
	}

	for _, attr := range attrs {
		var optValues []domain.AttrValueOption

		// Формуємо вираз для локалізованого label
		labelExpr := fmt.Sprintf("COALESCE(av.value_string->>'%s', CAST(av.value_numeric AS TEXT))", lang)

		err := applyOtherFilters(baseQuery.Session(&gorm.Session{}), []string{"attr:" + attr.Code}).
			Joins(`JOIN attribute_value av ON (
				(av.product_id = product_variation.product_id OR av.variation_id = product_variation.id)
				OR EXISTS (
					SELECT 1 FROM product_bundle_item pbi
					JOIN product_variation pv_comp ON pv_comp.id = pbi.variation_id
					WHERE pbi.bundle_id = product.id 
					  AND (av.product_id = pv_comp.product_id OR av.variation_id = pv_comp.id)
				)
			)`).
			Joins("JOIN attribute a ON a.id = av.attribute_id AND a.deleted_at IS NULL").
			Where("a.id = ?", attr.ID).
			Select("av.value_code as code, " + labelExpr + " as label, COUNT(DISTINCT product_variation.id) as count").
			Group("av.value_code, " + labelExpr).
			Order("label ASC").Scan(&optValues).Error

		if err == nil && len(optValues) > 0 {
			name := attr.Code
			for _, t := range attr.Translations {
				if t.LanguageCode == lang {
					name = t.Name
					break
				}
			}
			res.Attributes = append(res.Attributes, domain.AttributeFilter{
				ID:     attr.ID,
				Code:   attr.Code,
				Name:   name,
				Values: optValues,
			})
		} else if err != nil {
			r.l.Errorw("failed to count attribute values", "error", err, "attr_code", attr.Code)
		}
	}

	return &res, nil
}

func (r *productRepository) QuickSearch(ctx context.Context, queryStr string, lang string, limit int) ([]domain.QuickSearchProduct, error) {
	var results []domain.QuickSearchProduct

	// Валідація мови
	if lang != "uk" && lang != "en" {
		lang = "uk"
	}

	// Використовуємо підзапит, щоб отримати унікальні товари з основною або найдешевшою варіацією,
	// а потім сортуємо їх за релевантністю з урахуванням pg_trgm та ILIKE.
	subQuery := r.db.Table("product_variation pv").
		Select(`DISTINCT ON (p.id) p.id, 
			COALESCE(pt.name, pt_fallback.name, '') as name, 
			p.slug, pv.sku, pv.price, pv.old_price, pi.image_url`).
		Joins("JOIN product p ON p.id = pv.product_id AND p.deleted_at IS NULL AND p.is_active = true").
		Joins("LEFT JOIN product_translation pt ON pt.product_id = p.id AND pt.language_code = ?", lang).
		Joins("LEFT JOIN product_translation pt_fallback ON pt_fallback.product_id = p.id AND pt_fallback.language_code = 'uk'"). // Fallback to 'uk' which must exist
		Joins("LEFT JOIN product_image pi ON pi.product_id = p.id AND pi.is_primary = true AND (pi.variation_id IS NULL OR pi.variation_id = pv.id)").
		Where("pv.deleted_at IS NULL AND pv.is_active = true").
		Where(`(
			EXISTS (
				SELECT 1 FROM product_translation pt_search 
				WHERE pt_search.product_id = p.id 
				  AND pt_search.name ILIKE ?
			) 
			OR pv.sku ILIKE ?
		)`, "%"+queryStr+"%", "%"+queryStr+"%").
		Order("p.id, pv.price ASC")

	err := r.db.WithContext(ctx).Table("(?) as u", subQuery).
		Order(clause.Expr{SQL: `(
			EXISTS (
				SELECT 1 FROM product_translation pt_boost 
				WHERE pt_boost.product_id = u.id 
				  AND pt_boost.name ILIKE ? || '%'
			)
		) DESC, u.name ASC`, Vars: []interface{}{queryStr}}).
		Limit(limit).
		Scan(&results).Error

	if err != nil {
		r.l.Errorw("failed to execute quick search", "error", err, "query", queryStr)
		return nil, err
	}

	return results, nil
}

// enrichAttributeValueTranslations доповнює відсутні переклади дискретних значень
// (тих, що мають value_code) канонічними з уже наявних записів. Денормалізована
// модель зберігає value_string у кожному рядку attribute_value окремо; коли клієнт
// надсилає лише одну мову (напр. лише 'uk'), решту мов підтягуємо з будь-якого
// наявного значення з тим самим (attribute_id, value_code). Так тип на кшталт
// 'capsules' автоматично отримує переклад 'Capsules' для англійської.
func (r *productRepository) enrichAttributeValueTranslations(tx *gorm.DB, avs []domain.AttributeValue) error {
	for i := range avs {
		av := &avs[i]
		code := strings.TrimSpace(av.ValueCode)
		if code == "" {
			continue // без коду нема за чим підтягувати (вільний текст)
		}
		hasUk := av.ValueString != nil && strings.TrimSpace(av.ValueString["uk"]) != ""
		hasEn := av.ValueString != nil && strings.TrimSpace(av.ValueString["en"]) != ""
		if hasUk && hasEn {
			continue
		}

		var candidates []domain.AttributeValue
		if err := tx.Where("attribute_id = ? AND value_code = ? AND value_string IS NOT NULL", av.AttributeID, code).
			Limit(20).Find(&candidates).Error; err != nil {
			return err
		}
		for _, cand := range candidates {
			if cand.ValueString == nil {
				continue
			}
			if !hasUk {
				if v := strings.TrimSpace(cand.ValueString["uk"]); v != "" {
					if av.ValueString == nil {
						av.ValueString = domain.LocalizedMap{}
					}
					av.ValueString["uk"] = v
					hasUk = true
				}
			}
			if !hasEn {
				if v := strings.TrimSpace(cand.ValueString["en"]); v != "" {
					if av.ValueString == nil {
						av.ValueString = domain.LocalizedMap{}
					}
					av.ValueString["en"] = v
					hasEn = true
				}
			}
			if hasUk && hasEn {
				break
			}
		}
	}
	return nil
}

// enrichProductAttributeTranslations застосовує enrichAttributeValueTranslations
// до атрибутів товару та всіх його варіацій.
func (r *productRepository) enrichProductAttributeTranslations(tx *gorm.DB, product *domain.Product) error {
	if err := r.enrichAttributeValueTranslations(tx, product.AttributeValues); err != nil {
		return err
	}
	for i := range product.Variations {
		if err := r.enrichAttributeValueTranslations(tx, product.Variations[i].AttributeValues); err != nil {
			return err
		}
	}
	return nil
}

func (r *productRepository) Create(ctx context.Context, product *domain.Product) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.generateVariationSlugs(tx, product); err != nil {
			r.l.Errorw("failed to generate variation slugs", "error", err)
			return err
		}

		if err := r.enrichProductAttributeTranslations(tx, product); err != nil {
			r.l.Errorw("failed to enrich attribute translations", "error", err)
			return err
		}

		if err := tx.Create(product).Error; err != nil {
			r.l.Errorw("failed to create product", "error", err)
			return err
		}

		// Sync bundle items: GORM automatically saves them via association,
		// but we make sure the BundleID is set in the struct.
		if product.IsBundle && len(product.BundleItems) > 0 {
			for i := range product.BundleItems {
				product.BundleItems[i].BundleID = product.ID
			}
		}

		return nil
	})
}

func (r *productRepository) Update(ctx context.Context, product *domain.Product) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.generateVariationSlugs(tx, product); err != nil {
			r.l.Errorw("failed to generate variation slugs in update", "error", err, "id", product.ID)
			return err
		}

		if err := r.enrichProductAttributeTranslations(tx, product); err != nil {
			r.l.Errorw("failed to enrich attribute translations in update", "error", err, "id", product.ID)
			return err
		}

		// Update core fields only if they are provided
		updates := make(map[string]interface{})
		if product.BrandID != uuid.Nil {
			updates["brand_id"] = product.BrandID
		}
		if product.CategoryID != uuid.Nil {
			updates["category_id"] = product.CategoryID
		}
		// IsActive ми оновлюємо завжди, якщо він прийшов у запиті.
		// Оскільки в доменній моделі це bool, а в хендлері ми його вже перевірили,
		// ми можемо покластися на те, що хендлер передав потрібне значення.
		// Але щоб не затерти при PATCH {}, перевіримо чи він був у запиті (через хендлер це вже пройшло).
		updates["is_active"] = product.IsActive
		updates["is_bundle"] = product.IsBundle
		updates["is_recommended"] = product.IsRecommended
		updates["price_strategy"] = product.PriceStrategy

		if err := tx.Model(product).Omit(clause.Associations).Updates(updates).Error; err != nil {
			r.l.Errorw("failed to update product core fields", "error", err, "id", product.ID)
			return err
		}

		// Sync Translations (only if provided).
		// Важливо: Association(...).Replace() для has-many робить ON CONFLICT DO UPDATE лише по FK,
		// тож name/description/usage існуючих перекладів НЕ оновлювались (правки назви губились).
		// Тому робимо явний upsert з UpdateAll по складеному PK (product_id, language_code).
		if len(product.Translations) > 0 {
			var langCodes []string
			for i := range product.Translations {
				product.Translations[i].ProductID = product.ID
				langCodes = append(langCodes, product.Translations[i].LanguageCode)
			}
			// Видаляємо переклади мов, яких немає в новому списку.
			if err := tx.Where("product_id = ? AND language_code NOT IN ?", product.ID, langCodes).Delete(&domain.ProductTranslation{}).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "product_id"}, {Name: "language_code"}},
				UpdateAll: true,
			}).Create(&product.Translations).Error; err != nil {
				r.l.Errorw("failed to sync product translations", "error", err, "id", product.ID)
				return err
			}
		}

		// Sync Images (only if provided)
		if len(product.Images) > 0 {
			var imageIDs []uuid.UUID
			for _, img := range product.Images {
				if img.ID != uuid.Nil {
					imageIDs = append(imageIDs, img.ID)
				}
			}
			// Видаляємо картинки, які були видалені в сервісі
			if err := tx.Where("product_id = ? AND id NOT IN ?", product.ID, imageIDs).Delete(&domain.ProductImage{}).Error; err != nil {
				return err
			}
			if err := tx.Model(product).Association("Images").Replace(product.Images); err != nil {
				r.l.Errorw("failed to sync product images", "error", err, "id", product.ID)
				return err
			}
		}

		// Sync Product-level Attribute Values (only if provided)
		if len(product.AttributeValues) > 0 {
			var avIDs []uuid.UUID
			for _, av := range product.AttributeValues {
				if av.ID != uuid.Nil {
					avIDs = append(avIDs, av.ID)
				}
			}
			if err := tx.Where("product_id = ? AND id NOT IN ?", product.ID, avIDs).Delete(&domain.AttributeValue{}).Error; err != nil {
				return err
			}
			if err := tx.Model(product).Association("AttributeValues").Replace(product.AttributeValues); err != nil {
				r.l.Errorw("failed to sync product attribute values", "error", err, "id", product.ID)
				return err
			}
		}

		// Sync Product-level Badges (Full Sync if provided)
		if product.Badges != nil {
			if err := tx.Where("product_id = ? AND variation_id IS NULL", product.ID).Delete(&domain.ProductBadge{}).Error; err != nil {
				return err
			}
			if len(product.Badges) > 0 {
				if err := tx.Create(&product.Badges).Error; err != nil {
					r.l.Errorw("failed to create product badges", "error", err, "id", product.ID)
					return err
				}
			}
		}

		// Sync Variations (only if provided)
		if len(product.Variations) > 0 {
			var varIDs []uuid.UUID
			for _, v := range product.Variations {
				if v.ID != uuid.Nil {
					varIDs = append(varIDs, v.ID)
				}
			}
			// Важливо: ми використовуємо Unscoped для видалення варіацій, якщо хочемо їх реально видалити,
			// або звичайний Delete для Soft Delete. Тут ми робимо Soft Delete (через репозиторій).
			if err := tx.Where("product_id = ? AND id NOT IN ?", product.ID, varIDs).Delete(&domain.ProductVariation{}).Error; err != nil {
				return err
			}

			// Replace only updates foreign keys if they differ. To update scalar fields like is_active,
			// we must explicitly Save each variation.
			for _, v := range product.Variations {
				if err := tx.Omit(clause.Associations).Save(&v).Error; err != nil {
					r.l.Errorw("failed to save variation", "error", err, "variation_id", v.ID)
					return err
				}
			}

			if err := tx.Model(product).Association("Variations").Replace(product.Variations); err != nil {
				r.l.Errorw("failed to sync product variations", "error", err, "id", product.ID)
				return err
			}

			// Sync Variation-level Attribute Values
			for _, v := range product.Variations {
				var vAvIDs []uuid.UUID
				for _, av := range v.AttributeValues {
					if av.ID != uuid.Nil {
						vAvIDs = append(vAvIDs, av.ID)
					}
				}
				if len(vAvIDs) > 0 {
					if err := tx.Where("variation_id = ? AND id NOT IN ?", v.ID, vAvIDs).Delete(&domain.AttributeValue{}).Error; err != nil {
						return err
					}
				}
				if err := tx.Model(&v).Association("AttributeValues").Replace(v.AttributeValues); err != nil {
					r.l.Errorw("failed to sync variation attribute values", "error", err, "variation_id", v.ID)
					return err
				}

				// Sync Variation-level Badges (Full Sync if provided)
				if v.Badges != nil {
					if err := tx.Where("variation_id = ?", v.ID).Delete(&domain.ProductBadge{}).Error; err != nil {
						return err
					}
					if len(v.Badges) > 0 {
						if err := tx.Create(&v.Badges).Error; err != nil {
							r.l.Errorw("failed to create variation badges", "error", err, "variation_id", v.ID)
							return err
						}
					}
				}
			}
		}

		// Sync Bundle Items (Full Sync if product is a bundle)
		if product.IsBundle {
			// Delete old bundle items
			if err := tx.Where("bundle_id = ?", product.ID).Delete(&domain.ProductBundleItem{}).Error; err != nil {
				r.l.Errorw("failed to delete old bundle items", "error", err, "id", product.ID)
				return err
			}
			// Create new bundle items
			if len(product.BundleItems) > 0 {
				for i := range product.BundleItems {
					product.BundleItems[i].BundleID = product.ID
				}
				if err := tx.Create(&product.BundleItems).Error; err != nil {
					r.l.Errorw("failed to create bundle items", "error", err, "id", product.ID)
					return err
				}
			}
		}

		return nil
	})
}

func (r *productRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Soft delete variations first to maintain integrity
		if err := tx.Where("product_id = ?", id).Delete(&domain.ProductVariation{}).Error; err != nil {
			r.l.Errorw("failed to delete variations", "error", err, "product_id", id)
			return err
		}
		// Soft delete product
		if err := tx.Where("id = ?", id).Delete(&domain.Product{}).Error; err != nil {
			r.l.Errorw("failed to delete product", "error", err, "id", id)
			return err
		}
		return nil
	})
}
func (r *productRepository) CountByBrand(ctx context.Context, brandID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.Product{}).Where("brand_id = ?", brandID).Count(&count).Error
	return count, err
}

func (r *productRepository) CountByCategory(ctx context.Context, categoryID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.Product{}).Where("category_id = ?", categoryID).Count(&count).Error
	return count, err
}

func (r *productRepository) GetActiveCategoryIDs(ctx context.Context) (map[uuid.UUID]bool, error) {
	var ids []uuid.UUID
	err := r.db.WithContext(ctx).Model(&domain.Product{}).
		Where("is_active = ?", true).
		Distinct("category_id").
		Pluck("category_id", &ids).Error
	if err != nil {
		r.l.Errorw("failed to fetch active category ids", "error", err)
		return nil, err
	}

	result := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		result[id] = true
	}
	return result, nil
}

// --- Image Management ---

func (r *productRepository) FindImageByID(ctx context.Context, imageID uuid.UUID) (*domain.ProductImage, error) {
	var img domain.ProductImage
	if err := r.db.WithContext(ctx).Where("id = ?", imageID).First(&img).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrImageNotFound
		}
		return nil, err
	}
	return &img, nil
}

func (r *productRepository) DeleteImage(ctx context.Context, imageID uuid.UUID) error {
	result := r.db.WithContext(ctx).Unscoped().Where("id = ?", imageID).Delete(&domain.ProductImage{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrImageNotFound
	}
	return nil
}

func (r *productRepository) UpdateImage(ctx context.Context, image *domain.ProductImage) error {
	return r.db.WithContext(ctx).Save(image).Error
}

func (r *productRepository) ReorderImages(ctx context.Context, productID uuid.UUID, ids []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			if err := tx.Model(&domain.ProductImage{}).
				Where("id = ? AND product_id = ?", id, productID).
				Update("sort_order", i).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *productRepository) CreateImages(ctx context.Context, images []domain.ProductImage) error {
	if len(images) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&images).Error
}

func (r *productRepository) ResetImageRole(ctx context.Context, productID uuid.UUID, variationID *uuid.UUID, field string) error {
	query := r.db.WithContext(ctx).Model(&domain.ProductImage{}).Where("product_id = ?", productID)
	if variationID != nil {
		query = query.Where("variation_id = ?", *variationID)
	} else {
		query = query.Where("variation_id IS NULL")
	}
	return query.Update(field, false).Error
}

func parseQuantityFilterValue(qStr string) (float64, int, bool) {
	if strings.Contains(qStr, ":") {
		parts := strings.SplitN(qStr, ":", 2)
		if len(parts) == 2 {
			val, errVal := strconv.ParseFloat(parts[0], 64)
			unitID, errUnit := strconv.Atoi(parts[1])
			if errVal == nil && errUnit == nil {
				return val, unitID, true
			}
		}
	}
	val, err := strconv.ParseFloat(qStr, 64)
	if err == nil {
		return val, 0, false
	}
	return 0, 0, false
}

// AreVariationsNonBundle перевіряє, що всі зазначені варіації належать до звичайних товарів (is_bundle = false)
func (r *productRepository) AreVariationsNonBundle(ctx context.Context, variationIDs []uuid.UUID) (bool, error) {
	if len(variationIDs) == 0 {
		return true, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.ProductVariation{}).
		Joins("JOIN product ON product.id = product_variation.product_id").
		Where("product_variation.id IN ?", variationIDs).
		Where("product.is_bundle = ?", true).
		Count(&count).Error
	if err != nil {
		r.l.Errorw("failed to check variations for nested bundles", "error", err)
		return false, err
	}
	return count == 0, nil
}

func (r *productRepository) FindVariationsByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.ProductVariation, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var variations []domain.ProductVariation
	if err := r.db.WithContext(ctx).Preload("Product.Translations").Where("id IN ?", ids).Find(&variations).Error; err != nil {
		r.l.Errorw("failed to find variations by ids", "error", err, "ids", ids)
		return nil, err
	}
	return variations, nil
}

func (r *productRepository) FindAttributeByCode(ctx context.Context, code string) (*domain.Attribute, error) {
	var attr domain.Attribute
	if err := r.db.WithContext(ctx).Where("code = ?", code).First(&attr).Error; err != nil {
		return nil, err
	}
	return &attr, nil
}

// preloadBundleItems завантажує компоненти набору з повними даними варіацій та їхніх товарів
func (r *productRepository) preloadBundleItems(ctx context.Context, product *domain.Product, lang string) error {
	var items []domain.ProductBundleItem
	err := r.db.WithContext(ctx).
		Where("bundle_id = ?", product.ID).
		Preload("Variation", func(db *gorm.DB) *gorm.DB {
			return db.
				Preload("Unit").
				Preload("AttributeValues.Attribute.Translations", "language_code = ?", lang).
				Preload("AttributeValues.Attribute.Unit").
				Preload("AttributeValues.Unit").
				Preload("Badges", func(db *gorm.DB) *gorm.DB {
					return db.Preload("Badge")
				})
		}).
		Preload("Variation.Product", func(db *gorm.DB) *gorm.DB {
			return db.
				Preload("Translations").
				Preload("Brand").
				Preload("Category.Translations").
				Preload("Images", func(db *gorm.DB) *gorm.DB {
					return db.Order("sort_order asc")
				}).
				Preload("AttributeValues.Attribute.Translations", "language_code = ?", lang).
				Preload("AttributeValues.Attribute.Unit").
				Preload("AttributeValues.Unit")
		}).
		Find(&items).Error

	if err != nil {
		return err
	}
	product.BundleItems = items
	return nil
}

func getUnitSlugSuffix(unitShortNameUk string) string {
	switch strings.ToLower(unitShortNameUk) {
	case "шт":
		return "sht"
	case "кг":
		return "kg"
	case "мл":
		return "ml"
	case "л":
		return "l"
	case "г":
		return "g"
	default:
		// fallback to gosimple/slug
		return slugLib.Make(unitShortNameUk)
	}
}

func (r *productRepository) generateVariationSlugs(tx *gorm.DB, product *domain.Product) error {
	// 1. Fetch all units to resolve IDs to short_name
	var units []domain.Unit
	if err := tx.Find(&units).Error; err != nil {
		return err
	}
	unitMap := make(map[int]string)
	for _, u := range units {
		if ukShort, ok := u.ShortName["uk"]; ok && ukShort != "" {
			unitMap[u.ID] = ukShort
		} else if enShort, ok := u.ShortName["en"]; ok && enShort != "" {
			unitMap[u.ID] = enShort
		}
	}

	for i := range product.Variations {
		v := &product.Variations[i]

		var suffix string
		if v.QuantityValue > 0 && v.UnitID != nil {
			unitShort := unitMap[*v.UnitID]
			unitSuffix := getUnitSlugSuffix(unitShort)
			qtyStr := fmt.Sprintf("%g", v.QuantityValue)
			if unitSuffix != "" {
				suffix = qtyStr + "-" + unitSuffix
			} else {
				suffix = qtyStr
			}
		} else if v.Weight > 0 {
			weightStr := fmt.Sprintf("%g", v.Weight)
			weightStrClean := strings.ReplaceAll(weightStr, ".", "-")
			suffix = weightStrClean + "-kg"
		} else if v.SKU != "" {
			suffix = slugLib.Make(v.SKU)
		} else {
			idStr := v.ID.String()
			if len(idStr) > 8 {
				suffix = idStr[:8]
			} else {
				suffix = idStr
			}
		}

		prodSlug := "product"
		for _, t := range product.Translations {
			if t.LanguageCode == "uk" && t.Slug != "" {
				prodSlug = t.Slug
				break
			}
		}
		if prodSlug == "product" && len(product.Translations) > 0 {
			prodSlug = product.Translations[0].Slug
		}
		baseVarSlug := prodSlug + "-" + suffix
		candidate := baseVarSlug

		// Ensure uniqueness
		for attempt := 0; attempt < 100; attempt++ {
			if attempt > 0 {
				candidate = fmt.Sprintf("%s-%d", baseVarSlug, attempt)
			}

			var count int64
			err := tx.Model(&domain.ProductVariation{}).
				Where("slug = ? AND id != ?", candidate, v.ID).
				Count(&count).Error
			if err != nil {
				return err
			}

			var prodCount int64
			err = tx.Model(&domain.ProductTranslation{}).
				Where("slug = ?", candidate).
				Count(&prodCount).Error
			if err != nil {
				return err
			}

			duplicateInSlice := false
			for j := 0; j < i; j++ {
				if product.Variations[j].Slug == candidate {
					duplicateInSlice = true
					break
				}
			}

			if count == 0 && prodCount == 0 && !duplicateInSlice {
				v.Slug = candidate
				break
			}
		}
	}
	return nil
}

func (r *productRepository) FindRecommended(ctx context.Context, lang string, limit int) ([]domain.ProductVariation, error) {
	var variations []domain.ProductVariation

	// Select the default (cheapest) active variation of products flagged as recommended
	subQuery := r.db.Model(&domain.ProductVariation{}).
		Select("DISTINCT ON (product_variation.product_id) product_variation.id").
		Joins("JOIN product ON product.id = product_variation.product_id AND product.deleted_at IS NULL").
		Where("product.is_recommended = ? AND product.is_active = ? AND product_variation.is_active = ?", true, true, true).
		Order("product_variation.product_id, product_variation.price ASC")

	err := r.db.WithContext(ctx).Model(&domain.ProductVariation{}).
		Where("id IN (?)", subQuery).
		Preload("Product", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Translations").
				Preload("Brand").
				Preload("Category.Translations").
				Preload("Images", func(db *gorm.DB) *gorm.DB {
					return db.Order("sort_order asc")
				}).
				Preload("Badges", func(db *gorm.DB) *gorm.DB {
					return db.Preload("Badge")
				}).
				Preload("BundleItems")
		}).
		Preload("Badges", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Badge")
		}).
		Preload("Unit").
		Order("RANDOM()").
		Limit(limit).
		Find(&variations).Error

	return variations, err
}
