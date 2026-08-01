package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	slugLib "github.com/gosimple/slug"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/storage"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	redirectDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
)

type productService struct {
	repo     domain.ProductRepository
	storage  storage.Storage
	redirect redirectDomain.RedirectService
	cfg      *config.Config
	l        logger.Logger
}

const (
	BadgeIDNew        = 1
	BadgeIDBestSeller = 2
	BadgeIDSale       = 3
)

// NewProductService створює новий інстанс сервісу
func NewProductService(repo domain.ProductRepository, st storage.Storage, redirect redirectDomain.RedirectService, cfg *config.Config, l logger.Logger) domain.ProductService {
	return &productService{repo: repo, storage: st, redirect: redirect, cfg: cfg, l: l}
}

// GetRecommended отримує рандомізований список рекомендованих товарів
func (s *productService) GetRecommended(ctx context.Context, lang string, limit int) ([]domain.ProductVariation, error) {
	return s.repo.FindRecommended(ctx, lang, limit)
}

// GetList отримує список варіацій товарів, враховуючи фільтри, і формує метадані для пагінації
func (s *productService) GetList(ctx context.Context, lang string, filter domain.ProductFilter, pgn pagination.Params) ([]domain.ProductVariation, pagination.Metadata, error) {
	variations, totalItems, err := s.repo.FindAll(ctx, lang, filter, pgn)
	if err != nil {
		return nil, pagination.Metadata{}, err
	}

	metadata := pagination.CalculateMetadata(totalItems, pgn.Page, pgn.Limit)

	// Розраховуємо динамічні бейджі для кожної варіації
	for i := range variations {
		s.computeVariationBadges(&variations[i])
		// Додаємо product-level бейджі (Product preloaded у FindAll)
		if variations[i].Product.ID != uuid.Nil {
			s.computeProductBadges(&variations[i].Product)
			variations[i].ComputedBadges = append(variations[i].ComputedBadges, variations[i].Product.ComputedBadges...)
		}
	}

	return variations, metadata, nil
}

// GetByID отримує деталі конретного товару
func (s *productService) GetByID(ctx context.Context, id uuid.UUID, lang string) (*domain.Product, error) {
	product, err := s.repo.FindByID(ctx, id, lang)
	if err != nil {
		return nil, err
	}

	// Розраховуємо динамічні бейджі для товару
	s.computeProductBadges(product)

	// Розраховуємо динамічні бейджі для варіацій та додаємо бейджі товару
	for i := range product.Variations {
		s.computeVariationBadges(&product.Variations[i])
		// Додаємо product-level computed badges до кожної варіації
		product.Variations[i].ComputedBadges = append(product.Variations[i].ComputedBadges, product.ComputedBadges...)
	}

	// Динамічне ціноутворення для наборів
	if product.IsBundle {
		s.computeBundlePricing(product)
	}

	return product, nil
}

// GetBySlug отримує деталі товару за SEO slug
func (s *productService) GetBySlug(ctx context.Context, slug string, lang string) (*domain.Product, error) {
	product, err := s.repo.FindBySlug(ctx, slug, lang)
	if err != nil {
		return nil, err
	}

	// Розраховуємо динамічні бейджі для товару
	s.computeProductBadges(product)

	// Розраховуємо динамічні бейджі для варіацій та додаємо бейджі товару
	for i := range product.Variations {
		s.computeVariationBadges(&product.Variations[i])
		product.Variations[i].ComputedBadges = append(product.Variations[i].ComputedBadges, product.ComputedBadges...)
	}

	// Динамічне ціноутворення для наборів
	if product.IsBundle {
		s.computeBundlePricing(product)
	}

	return product, nil
}

func (s *productService) GenerateSlug(ctx context.Context, name string, excludeID uuid.UUID) (string, error) {
	baseSlug := slugLib.Make(name)
	if baseSlug == "" {
		baseSlug = "product"
	}

	candidate := baseSlug
	for attempts := 1; attempts <= 100; attempts++ {
		exists, err := s.repo.SlugExists(ctx, candidate, excludeID)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}

		// Колізія — додаємо порядковий номер
		candidate = fmt.Sprintf("%s-%d", baseSlug, attempts)
	}

	return candidate, nil
}

// GetReviews отримує підтверджені відгуки для товару
func (s *productService) GetReviews(ctx context.Context, productID uuid.UUID, pgn pagination.Params) ([]domain.ProductReview, pagination.Metadata, error) {
	// Спочатку перевіряємо, чи існує сам товар
	exists, err := s.repo.Exists(ctx, productID)
	if err != nil {
		return nil, pagination.Metadata{}, err
	}
	if !exists {
		return nil, pagination.Metadata{}, domain.ErrProductNotFound
	}

	reviews, totalItems, err := s.repo.FindReviews(ctx, productID, pgn)
	if err != nil {
		return nil, pagination.Metadata{}, err
	}

	metadata := pagination.CalculateMetadata(totalItems, pgn.Page, pgn.Limit)
	return reviews, metadata, nil
}

func (s *productService) GetPendingReviews(ctx context.Context, pgn pagination.Params) ([]domain.ProductReview, pagination.Metadata, error) {
	reviews, totalItems, err := s.repo.FindPendingReviews(ctx, pgn)
	if err != nil {
		return nil, pagination.Metadata{}, err
	}

	metadata := pagination.CalculateMetadata(totalItems, pgn.Page, pgn.Limit)
	return reviews, metadata, nil
}

func (s *productService) GetRejectedReviews(ctx context.Context, pgn pagination.Params) ([]domain.ProductReview, pagination.Metadata, error) {
	reviews, totalItems, err := s.repo.FindRejectedReviews(ctx, pgn)
	if err != nil {
		return nil, pagination.Metadata{}, err
	}
	return reviews, pagination.CalculateMetadata(totalItems, pgn.Page, pgn.Limit), nil
}

// AddReview валідує та створює відгук або відповідь
func (s *productService) AddReview(ctx context.Context, review *domain.ProductReview) error {
	isReply := review.ParentID != nil

	// Рейтинг перевіряємо лише для звичайних відгуків, не для відповідей
	if !isReply {
		if review.Rating < 1 || review.Rating > 5 {
			return domain.ErrInvalidRating
		}
	} else {
		// У відповідей рейтинг не має сенсу — зануляємо для чистоти БД
		review.Rating = 0
	}

	if len(review.Comment) > 2000 {
		return domain.ErrCommentTooLong
	}

	if review.ID == uuid.Nil {
		review.ID = uuid.New()
	}

	return s.repo.CreateReview(ctx, review)
}

// ApproveReview підтверджує відгук (викличе транзакцію в репозиторії)
func (s *productService) ApproveReview(ctx context.Context, reviewID uuid.UUID) error {
	return s.repo.ApproveReview(ctx, reviewID)
}

// RejectReview відхиляє відгук або залишає відхиленим, перераховуючи рейтинг (якщо відгук був approved)
func (s *productService) RejectReview(ctx context.Context, reviewID uuid.UUID, reason *string) error {
	return s.repo.RejectReview(ctx, reviewID, reason)
}

func (s *productService) ReopenReview(ctx context.Context, reviewID uuid.UUID) error {
	return s.repo.ReopenReview(ctx, reviewID)
}

// GetFilters завантажує всі доступні параметри фільтрації для категорії
func (s *productService) GetFilters(ctx context.Context, filter domain.ProductFilter, lang string) (*domain.FilterDiscovery, error) {
	return s.repo.GetFilters(ctx, filter, lang)
}

// GetUnits повертає всі одиниці виміру (для вибору місткості при створенні товару).
func (s *productService) GetUnits(ctx context.Context) ([]domain.Unit, error) {
	return s.repo.GetUnits(ctx)
}

// QuickSearch шукає товари для автодоповнення
func (s *productService) QuickSearch(ctx context.Context, query string, lang string, limit int) ([]domain.QuickSearchProduct, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []domain.QuickSearchProduct{}, nil
	}
	if limit <= 0 {
		limit = 5
	}
	return s.repo.QuickSearch(ctx, query, lang, limit)
}

// --- Admin CRUD ---

func (s *productService) CreateProduct(ctx context.Context, product *domain.Product, images []domain.ImageUpload) error {
	s.applyTranslationFallback(product)

	if product.ID == uuid.Nil {
		product.ID = uuid.New()
	}

	for i := range product.Translations {
		t := &product.Translations[i]
		if t.Slug == "" && t.Name != "" {
			slug, err := s.GenerateSlug(ctx, t.Name, product.ID)
			if err != nil {
				return fmt.Errorf("failed to generate slug: %w", err)
			}
			t.Slug = slug
		}
	}

	if len(product.Variations) == 0 {
		return fmt.Errorf("at least one variation is required")
	}

	for i := range product.Variations {
		v := &product.Variations[i]
		if v.Slug == "" {
			nameForSlug := ""
			if v.Name != nil && v.Name["uk"] != "" {
				nameForSlug = v.Name["uk"]
			} else {
				// Fallback до назви товару
				for _, t := range product.Translations {
					if t.LanguageCode == "uk" {
						nameForSlug = t.Name
						break
					}
				}
			}
			if nameForSlug != "" {
				slug, err := s.GenerateSlug(ctx, nameForSlug, v.ID)
				if err == nil {
					v.Slug = slug
				}
			}
		}
	}

	// Валідація набору: перевірка на вкладені набори
	if product.IsBundle {
		if len(product.BundleItems) == 0 {
			return domain.ErrBundleNoComponents
		}
		varIDs := make([]uuid.UUID, len(product.BundleItems))
		for i, item := range product.BundleItems {
			varIDs[i] = item.VariationID
		}
		ok, err := s.repo.AreVariationsNonBundle(ctx, varIDs)
		if err != nil {
			return fmt.Errorf("failed to validate bundle components: %w", err)
		}
		if !ok {
			return domain.ErrNestedBundle
		}

		// Завантажуємо дані варіацій компонентів для розрахунку ціни та ваги
		vars, err := s.repo.FindVariationsByIDs(ctx, varIDs)
		if err != nil {
			return fmt.Errorf("failed to fetch bundle component variations: %w", err)
		}
		varMap := make(map[uuid.UUID]domain.ProductVariation)
		for _, v := range vars {
			varMap[v.ID] = v
		}
		for i := range product.BundleItems {
			if v, ok := varMap[product.BundleItems[i].VariationID]; ok {
				product.BundleItems[i].Variation = v
			}
		}
		s.computeBundlePricing(product)
		if err := s.computeAndSetBundleAttributes(ctx, product); err != nil {
			return fmt.Errorf("failed to compute bundle attributes: %w", err)
		}
	}

	if err := s.uploadImages(ctx, product, images); err != nil {
		return err
	}

	if err := s.repo.Create(ctx, product); err != nil {
		// Rollback: delete uploaded files from Cloudinary
		s.l.Errorw("failed to save product to DB, rolling back cloudinary uploads", "error", err, "product_id", product.ID)
		go func(imgs []domain.ProductImage) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, img := range imgs {
				if publicID := extractPublicID(img.ImageURL); publicID != "" {
					_ = s.storage.Delete(ctx, publicID)
				}
			}
		}(product.Images)
		return err
	}

	return nil
}

func (s *productService) UpdateProduct(ctx context.Context, product *domain.Product, variationsUpdate *[]domain.ProductVariationUpdate, newImages []domain.ImageUpload, imagesToDelete []uuid.UUID, isActive *bool, isBundle *bool, attributeValuesProvided bool) error {
	s.applyTranslationFallback(product)

	// 1. Get existing product to have all current data
	existing, err := s.repo.FindByID(ctx, product.ID, "uk")
	if err != nil {
		return fmt.Errorf("failed to get existing product: %w", err)
	}

	// 2. Merge core fields
	if product.BrandID != uuid.Nil {
		existing.BrandID = product.BrandID
	}
	if product.CategoryID != uuid.Nil {
		existing.CategoryID = product.CategoryID
	}

	if isActive != nil {
		existing.IsActive = *isActive
	}

	// 2.2 Merge bundle fields
	if isBundle != nil {
		existing.IsBundle = *isBundle
	}
	if product.PriceStrategy != "" {
		existing.PriceStrategy = product.PriceStrategy
	}
	if product.BundleItems != nil {
		existing.BundleItems = product.BundleItems
	}

	// Валідація набору: перевірка на вкладені набори
	if existing.IsBundle && len(existing.BundleItems) > 0 {
		varIDs := make([]uuid.UUID, len(existing.BundleItems))
		for i, item := range existing.BundleItems {
			varIDs[i] = item.VariationID
		}
		ok, err := s.repo.AreVariationsNonBundle(ctx, varIDs)
		if err != nil {
			return fmt.Errorf("failed to validate bundle components: %w", err)
		}
		if !ok {
			return domain.ErrNestedBundle
		}

		// Завантажуємо дані варіацій компонентів для розрахунку ціни та ваги
		vars, err := s.repo.FindVariationsByIDs(ctx, varIDs)
		if err != nil {
			return fmt.Errorf("failed to fetch bundle component variations: %w", err)
		}
		varMap := make(map[uuid.UUID]domain.ProductVariation)
		for _, v := range vars {
			varMap[v.ID] = v
		}
		for i := range existing.BundleItems {
			if v, ok := varMap[existing.BundleItems[i].VariationID]; ok {
				existing.BundleItems[i].Variation = v
			}
		}
	}

	// 2.1 Merge badges — Full Sync
	if product.Badges != nil {
		existing.Badges = product.Badges
	}

	// Check if name changed before merging translations
	// nameChanged is removed since we only generate slug on empty.

	// 3. Merge translations — завжди перезаписуємо передані поля (навіть порожні рядки)
	if len(product.Translations) > 0 {
		for _, newT := range product.Translations {
			found := false
			for i := range existing.Translations {
				if existing.Translations[i].LanguageCode == newT.LanguageCode {
					oldName := existing.Translations[i].Name
					oldSlug := existing.Translations[i].Slug

					existing.Translations[i].Name = newT.Name
					existing.Translations[i].Description = newT.Description
					existing.Translations[i].UsageInstructions = newT.UsageInstructions
					existing.Translations[i].MetaTitle = newT.MetaTitle
					existing.Translations[i].MetaDescription = newT.MetaDescription
					existing.Translations[i].MetaKeywords = newT.MetaKeywords

					if newT.Name != "" && oldName != newT.Name {
						newSlug, err := s.GenerateSlug(ctx, newT.Name, existing.ID)
						if err != nil {
							return fmt.Errorf("failed to generate slug: %w", err)
						}
						existing.Translations[i].Slug = newSlug

						if oldSlug != "" && oldSlug != newSlug {
							_ = s.redirect.RecordSlugChange(ctx, "product", existing.ID, oldSlug)
						}
					}

					found = true
					break
				}
			}
			if !found {
				newT.ProductID = existing.ID
				if newT.Slug == "" && newT.Name != "" {
					slug, err := s.GenerateSlug(ctx, newT.Name, existing.ID)
					if err != nil {
						return fmt.Errorf("failed to generate slug: %w", err)
					}
					newT.Slug = slug
				}
				existing.Translations = append(existing.Translations, newT)
			}
		}
		s.applyTranslationFallback(existing)
	}

	// Regenerate slug if needed
	for i := range existing.Translations {
		t := &existing.Translations[i]
		if t.Slug == "" && t.Name != "" {
			slug, err := s.GenerateSlug(ctx, t.Name, existing.ID)
			if err != nil {
				return fmt.Errorf("failed to generate slug: %w", err)
			}
			t.Slug = slug
		}
	}

	// 4. Full Sync атрибутів товару
	if attributeValuesProvided {
		// Навіть якщо масив порожній — очищаємо все
		for i := range product.AttributeValues {
			product.AttributeValues[i].ProductID = &existing.ID
			product.AttributeValues[i].VariationID = nil
		}
		existing.AttributeValues = product.AttributeValues
	}

	// 4.1 Full Sync бейджів товару
	if product.Badges != nil {
		for i := range product.Badges {
			product.Badges[i].ProductID = &existing.ID
		}
		existing.Badges = product.Badges
	}

	// 5. Prepare images to delete from Cloudinary
	var cloudinaryImagesToDelete []string
	for _, idToDelete := range imagesToDelete {
		for _, img := range existing.Images {
			if img.ID == idToDelete {
				if publicID := extractPublicID(img.ImageURL); publicID != "" {
					cloudinaryImagesToDelete = append(cloudinaryImagesToDelete, publicID)
				}
				break
			}
		}
	}

	// 6. Keep images that are not deleted
	var remainingImages []domain.ProductImage
	for _, img := range existing.Images {
		deleted := false
		for _, idToDelete := range imagesToDelete {
			if img.ID == idToDelete {
				deleted = true
				break
			}
		}
		if !deleted {
			remainingImages = append(remainingImages, img)
		}
	}
	existing.Images = remainingImages

	// 7. Full Sync варіацій (list) with Partial Update (fields)
	if variationsUpdate != nil {
		// Побудувати map нових ID
		newVarIDSet := make(map[uuid.UUID]bool)
		for _, newV := range *variationsUpdate {
			if newV.ID != nil && *newV.ID != uuid.Nil {
				newVarIDSet[*newV.ID] = true
			}
		}

		// Фільтруємо existing.Variations — залишаємо лише ті, що є в новому списку
		var keptVariations []domain.ProductVariation
		for _, existingV := range existing.Variations {
			if newVarIDSet[existingV.ID] {
				keptVariations = append(keptVariations, existingV)
			}
			// Варіації, яких немає в новому списку, будуть видалені через репозиторій (soft delete)
		}

		// Для кожної нової варіації: оновлюємо existing або додаємо як нову
		for _, newV := range *variationsUpdate {
			found := false
			for i := range keptVariations {
				if newV.ID != nil && keptVariations[i].ID == *newV.ID {
					// Часткове оновлення полів (через вказівники)
					if newV.Name != nil {
						oldName := keptVariations[i].Name
						oldSlug := keptVariations[i].Slug
						keptVariations[i].Name = *newV.Name

						oldNameUk := oldName["uk"]
						newNameUk := (*newV.Name)["uk"]

						if newNameUk != "" && oldNameUk != newNameUk {
							newSlug, err := s.GenerateSlug(ctx, newNameUk, keptVariations[i].ID)
							if err != nil {
								return fmt.Errorf("failed to generate slug for variation: %w", err)
							}
							keptVariations[i].Slug = newSlug

							if oldSlug != "" && oldSlug != newSlug {
								_ = s.redirect.RecordSlugChange(ctx, "product_variation", keptVariations[i].ID, oldSlug)
							}
						}
					}
					if newV.SKU != nil {
						keptVariations[i].SKU = *newV.SKU
					}
					if newV.Barcode != nil {
						keptVariations[i].Barcode = *newV.Barcode
					}
					if newV.Price != nil {
						keptVariations[i].Price = *newV.Price
					}
					if newV.OldPrice != nil {
						keptVariations[i].OldPrice = newV.OldPrice
					}
					if newV.QuantityValue != nil {
						keptVariations[i].QuantityValue = *newV.QuantityValue
					}
					if newV.Weight != nil {
						keptVariations[i].Weight = *newV.Weight
					}
					if newV.UnitID != nil {
						keptVariations[i].UnitID = newV.UnitID
					}
					if newV.IsActive != nil {
						keptVariations[i].IsActive = *newV.IsActive
					}

					// Характеристики варіації — Full Sync (тільки якщо передано)
					if newV.AttributeValues != nil {
						for j := range *newV.AttributeValues {
							(*newV.AttributeValues)[j].VariationID = &keptVariations[i].ID
						}
						keptVariations[i].AttributeValues = *newV.AttributeValues
					}

					// Бейджі варіації — Full Sync (тільки якщо передано)
					if newV.BadgeIDs != nil {
						badges := make([]domain.ProductBadge, len(*newV.BadgeIDs))
						for j, bid := range *newV.BadgeIDs {
							badges[j] = domain.ProductBadge{
								VariationID: &keptVariations[i].ID,
								BadgeID:     bid,
							}
						}
						keptVariations[i].Badges = badges
					}

					found = true
					break
				}
			}
			if !found {
				// Створюємо нову варіацію
				newVariation := domain.ProductVariation{
					ID:        uuid.New(),
					ProductID: existing.ID,
					IsActive:  true, // default for new
				}
				if newV.Name != nil {
					newVariation.Name = *newV.Name
				}
				if newV.ID != nil && *newV.ID != uuid.Nil {
					newVariation.ID = *newV.ID
				}
				if newV.SKU != nil {
					newVariation.SKU = *newV.SKU
				}
				if newV.Barcode != nil {
					newVariation.Barcode = *newV.Barcode
				}
				if newV.Price != nil {
					newVariation.Price = *newV.Price
				}
				if newV.OldPrice != nil {
					newVariation.OldPrice = newV.OldPrice
				}
				if newV.QuantityValue != nil {
					newVariation.QuantityValue = *newV.QuantityValue
				}
				if newV.Weight != nil {
					newVariation.Weight = *newV.Weight
				} else {
					newVariation.Weight = 0.5 // Default weight
				}

				// Згенеруємо slug для нової варіації
				nameForSlug := ""
				if newVariation.Name != nil && newVariation.Name["uk"] != "" {
					nameForSlug = newVariation.Name["uk"]
				} else {
					// Fallback до назви товару
					for _, t := range existing.Translations {
						if t.LanguageCode == "uk" {
							nameForSlug = t.Name
							break
						}
					}
				}
				if nameForSlug != "" {
					slug, err := s.GenerateSlug(ctx, nameForSlug, newVariation.ID)
					if err == nil {
						newVariation.Slug = slug
					}
				}

				if newV.UnitID != nil {
					newVariation.UnitID = newV.UnitID
				}
				if newV.IsActive != nil {
					newVariation.IsActive = *newV.IsActive
				}

				if newV.AttributeValues != nil {
					for j := range *newV.AttributeValues {
						(*newV.AttributeValues)[j].VariationID = &newVariation.ID
					}
					newVariation.AttributeValues = *newV.AttributeValues
				}

				// Бейджі для нової варіації
				if newV.BadgeIDs != nil {
					badges := make([]domain.ProductBadge, len(*newV.BadgeIDs))
					for j, bid := range *newV.BadgeIDs {
						badges[j] = domain.ProductBadge{
							VariationID: &newVariation.ID,
							BadgeID:     bid,
						}
					}
					newVariation.Badges = badges
				}

				keptVariations = append(keptVariations, newVariation)
			}
		}

		existing.Variations = keptVariations
	}

	// 8. Upload new images (appends to existing.Images)
	originalImagesCount := len(existing.Images)
	if err := s.uploadImages(ctx, existing, newImages); err != nil {
		return fmt.Errorf("failed to upload new images: %w", err)
	}
	uploadedImages := existing.Images[originalImagesCount:]

	// Розраховуємо динамічні ціни та вагу для набору перед оновленням у БД
	if existing.IsBundle {
		s.computeBundlePricing(existing)
		if err := s.computeAndSetBundleAttributes(ctx, existing); err != nil {
			return fmt.Errorf("failed to compute bundle attributes: %w", err)
		}
	}

	// 9. Save updated product
	if err := s.repo.Update(ctx, existing); err != nil {
		// Rollback: delete NEWLY uploaded files from Cloudinary
		go func(imgs []domain.ProductImage) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, img := range imgs {
				if publicID := extractPublicID(img.ImageURL); publicID != "" {
					_ = s.storage.Delete(ctx, publicID)
				}
			}
		}(uploadedImages)
		return err
	}

	// 10. If DB update successful, FINALLY delete old images from Cloudinary
	if len(cloudinaryImagesToDelete) > 0 {
		go func(ids []string) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, pid := range ids {
				_ = s.storage.Delete(ctx, pid)
			}
		}(cloudinaryImagesToDelete)
	}

	return nil
}

func (s *productService) DeleteProduct(ctx context.Context, id uuid.UUID) error {
	// Перевіряємо існування товару перед видаленням
	exists, err := s.repo.Exists(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return domain.ErrProductNotFound
	}

	// Ми не видаляємо картинки з Cloudinary тут, оскільки товар видаляється м'яко (soft delete).
	// Це важливо для збереження історії замовлень користувачів.
	return s.repo.Delete(ctx, id)
}

// --- Image Management ---

func (s *productService) UploadProductImages(ctx context.Context, productID uuid.UUID, images []domain.ImageUpload) ([]domain.ProductImage, error) {
	if len(images) == 0 {
		return nil, nil
	}

	// Перевіряємо існування товару
	exists, err := s.repo.Exists(ctx, productID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrProductNotFound
	}

	// Завантажуємо в Cloudinary
	productFolder := fmt.Sprintf("products/%s", productID.String())
	var result []domain.ProductImage

	for _, imgInfo := range images {
		url, err := s.storage.Upload(ctx, imgInfo.Content, productFolder, imgInfo.Filename)
		if err != nil {
			// Rollback вже завантажених
			go func(imgs []domain.ProductImage) {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				for _, img := range imgs {
					if publicID := extractPublicID(img.ImageURL); publicID != "" {
						_ = s.storage.Delete(ctx, publicID)
					}
				}
			}(result)
			return nil, fmt.Errorf("failed to upload image %s: %w", imgInfo.Filename, err)
		}
		img := domain.ProductImage{
			ID:          uuid.New(),
			ProductID:   productID,
			ImageURL:    url,
			IsPrimary:   imgInfo.IsPrimary,
			IsHover:     imgInfo.IsHover,
			SortOrder:   imgInfo.SortOrder,
			VariationID: imgInfo.VariationID,
		}
		result = append(result, img)
	}

	// Скидаємо ролі is_primary/is_hover у існуючих зображеннях, щоб уникнути дублікатів
	if err := s.resetImageRolesForBatch(ctx, productID, result); err != nil {
		go func(imgs []domain.ProductImage) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, img := range imgs {
				if publicID := extractPublicID(img.ImageURL); publicID != "" {
					_ = s.storage.Delete(ctx, publicID)
				}
			}
		}(result)
		return nil, fmt.Errorf("failed to reset image roles: %w", err)
	}

	// Зберігаємо в БД
	if err := s.repo.CreateImages(ctx, result); err != nil {
		// Rollback Cloudinary
		go func(imgs []domain.ProductImage) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, img := range imgs {
				if publicID := extractPublicID(img.ImageURL); publicID != "" {
					_ = s.storage.Delete(ctx, publicID)
				}
			}
		}(result)
		return nil, fmt.Errorf("failed to save images to DB: %w", err)
	}

	return result, nil
}

func (s *productService) DeleteProductImage(ctx context.Context, productID uuid.UUID, imageID uuid.UUID) error {
	img, err := s.repo.FindImageByID(ctx, imageID)
	if err != nil {
		return err
	}

	// Перевіряємо, що фото належить цьому товару
	if img.ProductID != productID {
		return domain.ErrImageNotFound
	}

	// Видаляємо з БД
	if err := s.repo.DeleteImage(ctx, imageID); err != nil {
		return err
	}

	// Видаляємо з Cloudinary (async, не блокуємо відповідь)
	go func(url string) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if publicID := extractPublicID(url); publicID != "" {
			_ = s.storage.Delete(ctx, publicID)
		}
	}(img.ImageURL)

	return nil
}

func (s *productService) UpdateProductImage(ctx context.Context, productID uuid.UUID, imageID uuid.UUID, isPrimary, isHover *bool, sortOrder *int, variationIDSet *bool, variationID *uuid.UUID, altTextUk, altTextEn *string) error {
	img, err := s.repo.FindImageByID(ctx, imageID)
	if err != nil {
		return err
	}

	if img.ProductID != productID {
		return domain.ErrImageNotFound
	}

	// 1. Спочатку змінюємо область, якщо прапорець встановлений
	if variationIDSet != nil && *variationIDSet {
		img.VariationID = variationID
	}

	// 2. Якщо встановлюємо is_primary — скидаємо у інших фото в поточній (можливо вже зміненій) області
	if isPrimary != nil && *isPrimary {
		if err := s.repo.ResetImageRole(ctx, productID, img.VariationID, "is_primary"); err != nil {
			return err
		}
		img.IsPrimary = true
	} else if isPrimary != nil {
		img.IsPrimary = false
	}

	// Аналогічно для is_hover
	if isHover != nil && *isHover {
		if err := s.repo.ResetImageRole(ctx, productID, img.VariationID, "is_hover"); err != nil {
			return err
		}
		img.IsHover = true
	} else if isHover != nil {
		img.IsHover = false
	}

	if sortOrder != nil {
		img.SortOrder = *sortOrder
	}

	// Локалізований alt-текст: оновлюємо лише передані мови, решту лишаємо як є.
	if altTextUk != nil || altTextEn != nil {
		if img.AltText == nil {
			img.AltText = domain.LocalizedMap{}
		}
		if altTextUk != nil {
			img.AltText["uk"] = *altTextUk
		}
		if altTextEn != nil {
			img.AltText["en"] = *altTextEn
		}
	}

	return s.repo.UpdateImage(ctx, img)
}

func (s *productService) ReorderProductImages(ctx context.Context, productID uuid.UUID, ids []uuid.UUID) error {
	return s.repo.ReorderImages(ctx, productID, ids)
}

// Helpers

func (s *productService) applyTranslationFallback(product *domain.Product) {
	var ukT, enT *domain.ProductTranslation
	for i := range product.Translations {
		t := &product.Translations[i]
		if t.LanguageCode == "uk" {
			ukT = t
		}
		if t.LanguageCode == "en" {
			enT = t
		}
	}

	if ukT != nil && enT != nil {
		if enT.Name == "" {
			enT.Name = ukT.Name
		}
		if enT.Description == "" {
			enT.Description = ukT.Description
		}
		if enT.UsageInstructions == "" {
			enT.UsageInstructions = ukT.UsageInstructions
		}
	}
}

func (s *productService) uploadImages(ctx context.Context, product *domain.Product, images []domain.ImageUpload) error {
	if len(images) == 0 {
		return nil
	}

	productFolder := fmt.Sprintf("products/%s", product.ID.String())
	for _, imgInfo := range images {
		url, err := s.storage.Upload(ctx, imgInfo.Content, productFolder, imgInfo.Filename)
		if err != nil {
			return fmt.Errorf("failed to upload image %s: %w", imgInfo.Filename, err)
		}
		img := domain.ProductImage{
			ID:        uuid.New(),
			ProductID: product.ID,
			ImageURL:  url,
			IsPrimary: imgInfo.IsPrimary,
			IsHover:   imgInfo.IsHover,
			SortOrder: imgInfo.SortOrder,
		}
		img.VariationID = imgInfo.VariationID

		// Скидаємо роль у вже доданих зображеннях (in-memory), щоб уникнути дублікатів
		if img.IsPrimary || img.IsHover {
			for j := range product.Images {
				if sameVariationScope(product.Images[j].VariationID, img.VariationID) {
					if img.IsPrimary {
						product.Images[j].IsPrimary = false
					}
					if img.IsHover {
						product.Images[j].IsHover = false
					}
				}
			}
		}

		product.Images = append(product.Images, img)
	}
	return nil
}

func extractPublicID(url string) string {
	idx := strings.Index(url, "/products/")
	if idx == -1 {
		return ""
	}
	path := url[idx+1:]
	extIdx := strings.LastIndex(path, ".")
	if extIdx != -1 {
		path = path[:extIdx]
	}
	return path
}

// sameVariationScope перевіряє, чи два вказівники на variation_id вказують на одну й ту ж область:
// обидва nil (product-level) або обидва вказують на один UUID.
func sameVariationScope(a, b *uuid.UUID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// resetImageRolesForBatch скидає is_primary/is_hover у існуючих зображеннях БД
// та дедуплікує ролі всередині батчу (залишає роль лише останньому зображенню).
func (s *productService) resetImageRolesForBatch(ctx context.Context, productID uuid.UUID, images []domain.ProductImage) error {
	type scopeKey struct {
		hasVariation bool
		variationID  uuid.UUID
	}
	toKey := func(vid *uuid.UUID) scopeKey {
		if vid == nil {
			return scopeKey{}
		}
		return scopeKey{true, *vid}
	}

	// 1. Скидаємо ролі в БД (один раз на scope)
	primaryDone := make(map[scopeKey]bool)
	hoverDone := make(map[scopeKey]bool)
	for _, img := range images {
		k := toKey(img.VariationID)
		if img.IsPrimary && !primaryDone[k] {
			if err := s.repo.ResetImageRole(ctx, productID, img.VariationID, "is_primary"); err != nil {
				return fmt.Errorf("failed to reset is_primary: %w", err)
			}
			primaryDone[k] = true
		}
		if img.IsHover && !hoverDone[k] {
			if err := s.repo.ResetImageRole(ctx, productID, img.VariationID, "is_hover"); err != nil {
				return fmt.Errorf("failed to reset is_hover: %w", err)
			}
			hoverDone[k] = true
		}
	}

	// 2. Дедуплікація всередині батчу: залишаємо роль лише останньому зображенню
	lastPrimary := make(map[scopeKey]int)
	lastHover := make(map[scopeKey]int)
	for i, img := range images {
		k := toKey(img.VariationID)
		if img.IsPrimary {
			lastPrimary[k] = i
		}
		if img.IsHover {
			lastHover[k] = i
		}
	}
	for i := range images {
		k := toKey(images[i].VariationID)
		if images[i].IsPrimary {
			if last := lastPrimary[k]; last != i {
				images[i].IsPrimary = false
			}
		}
		if images[i].IsHover {
			if last := lastHover[k]; last != i {
				images[i].IsHover = false
			}
		}
	}

	return nil
}

func (s *productService) computeProductBadges(p *domain.Product) {
	if p == nil {
		return
	}

	// 1. Новинка (по даті створення продукту)
	if s.cfg.BadgeNewDays > 0 && time.Since(p.CreatedAt) < time.Duration(s.cfg.BadgeNewDays)*24*time.Hour {
		p.ComputedBadges = append(p.ComputedBadges, domain.Badge{
			ID:       BadgeIDNew,
			Name:     domain.LocalizedMap{"uk": "Новинка", "en": "New"},
			ColorHex: "#3B82F6",
		})
	}

	// 2. Хіт продажів (по кількості відгуків як тимчасовий показник до появи модуля замовлень)
	if s.cfg.BadgeBestSellerThreshold > 0 && p.ReviewsCount >= s.cfg.BadgeBestSellerThreshold {
		p.ComputedBadges = append(p.ComputedBadges, domain.Badge{
			ID:       BadgeIDBestSeller,
			Name:     domain.LocalizedMap{"uk": "Хіт продажів", "en": "Best Seller"},
			ColorHex: "#F59E0B",
		})
	}
}

func (s *productService) computeVariationBadges(v *domain.ProductVariation) {
	if v == nil {
		return
	}

	// Акція / Знижка (по ціні варіації)
	if v.OldPrice != nil && *v.OldPrice > v.Price && v.Price > 0 {
		discount := ((*v.OldPrice - v.Price) * 100) / *v.OldPrice
		if discount > 0 {
			v.ComputedBadges = append(v.ComputedBadges, domain.Badge{
				ID: BadgeIDSale,
				Name: domain.LocalizedMap{
					"uk": fmt.Sprintf("Знижка %d%%", discount),
					"en": fmt.Sprintf("Sale %d%%", discount),
				},
				ColorHex: "#EF4444",
			})
		}
	}
}

// UploadMedia завантажує файл у вказану папку Cloudinary (для загальних цілей, наприклад, іконок)
func (s *productService) UploadMedia(ctx context.Context, file interface{}, folder, filename string) (string, error) {
	if folder == "" {
		folder = "general"
	}
	url, err := s.storage.Upload(ctx, file, folder, filename)
	if err != nil {
		s.l.Errorw("failed to upload media", "folder", folder, "filename", filename, "error", err)
		return "", fmt.Errorf("failed to upload media: %w", err)
	}
	return url, nil
}

// computeBundlePricing розраховує динамічне ціноутворення та вагу для набору.
// - При стратегії "dynamic": ціна варіації набору = сума (ціна компонента * кількість).
// - При стратегії "manual": OldPrice (якщо не задана вручну) = сума компонентів, щоб показати економію.
// - Вага розраховується як сума ваг компонентів + 200г, заокруглена до більшої половини.
func (s *productService) computeBundlePricing(product *domain.Product) {
	if len(product.BundleItems) == 0 {
		return
	}

	// Обчислюємо суму компонентів та вагу
	componentsTotal := 0
	var totalWeight float64 = 0
	for _, item := range product.BundleItems {
		componentsTotal += item.Variation.Price * item.Quantity
		totalWeight += item.Variation.Weight * float64(item.Quantity)
	}

	// Вага набору = сума ваг компонентів × кількість (без штучного пакування/заокруглення).
	for i := range product.Variations {
		v := &product.Variations[i]

		// Записуємо розраховану вагу
		v.Weight = totalWeight

		if product.PriceStrategy == domain.PriceStrategyDynamic {
			// Динамічна ціна: ціна набору = сума компонентів
			v.Price = componentsTotal
		} else {
			// Мануальна ціна: OldPrice = сума компонентів (якщо не задана вручну),
			// щоб показати різницю/економію для клієнта
			if v.OldPrice == nil && componentsTotal > v.Price {
				v.OldPrice = &componentsTotal
			}
		}
	}
}

func (s *productService) computeAndSetBundleAttributes(ctx context.Context, product *domain.Product) error {
	if !product.IsBundle {
		return nil
	}

	countAttr, err := s.repo.FindAttributeByCode(ctx, "bundle_items_count")
	if err != nil {
		s.l.Warnw("bundle_items_count attribute not found in DB, skipping bundle attributes calculation", "error", err)
		return nil
	}
	itemsAttr, err := s.repo.FindAttributeByCode(ctx, "bundle_items")
	if err != nil {
		s.l.Warnw("bundle_items attribute not found in DB, skipping bundle attributes calculation", "error", err)
		return nil
	}

	// Calculate total quantity
	var totalQty float64 = 0
	for _, bi := range product.BundleItems {
		totalQty += float64(bi.Quantity)
	}

	// Calculate localized item list strings
	var ukItems []string
	var enItems []string
	for _, bi := range product.BundleItems {
		var prodNameUk string
		var prodNameEn string
		for _, t := range bi.Variation.Product.Translations {
			if t.LanguageCode == "uk" {
				prodNameUk = t.Name
			} else if t.LanguageCode == "en" {
				prodNameEn = t.Name
			}
		}
		// fallbacks
		if prodNameUk == "" && len(bi.Variation.Product.Translations) > 0 {
			prodNameUk = bi.Variation.Product.Translations[0].Name
		}
		if prodNameUk == "" {
			prodNameUk = bi.Variation.Product.ID.String()
		}
		if prodNameEn == "" && len(bi.Variation.Product.Translations) > 0 {
			prodNameEn = bi.Variation.Product.Translations[0].Name
		}
		if prodNameEn == "" {
			prodNameEn = bi.Variation.Product.ID.String()
		}

		ukItems = append(ukItems, fmt.Sprintf("%s (%d шт.)", prodNameUk, bi.Quantity))
		enItems = append(enItems, fmt.Sprintf("%s (%d pcs)", prodNameEn, bi.Quantity))
	}

	localizedItems := domain.LocalizedMap{
		"uk": strings.Join(ukItems, "\n"),
		"en": strings.Join(enItems, "\n"),
	}

	// Filter out any existing bundle attribute values
	var filteredAVs []domain.AttributeValue
	for _, av := range product.AttributeValues {
		if av.AttributeID != countAttr.ID && av.AttributeID != itemsAttr.ID {
			filteredAVs = append(filteredAVs, av)
		}
	}

	// Append new bundle attribute values
	product.AttributeValues = append(filteredAVs,
		domain.AttributeValue{
			ID:           uuid.New(),
			AttributeID:  countAttr.ID,
			ValueNumeric: &totalQty,
			UnitID:       countAttr.UnitID,
			Unit:         countAttr.Unit,
			ProductID:    &product.ID,
		},
		domain.AttributeValue{
			ID:          uuid.New(),
			AttributeID: itemsAttr.ID,
			ValueString: localizedItems,
			UnitID:      itemsAttr.UnitID,
			Unit:        itemsAttr.Unit,
			ProductID:   &product.ID,
		},
	)
	return nil
}
