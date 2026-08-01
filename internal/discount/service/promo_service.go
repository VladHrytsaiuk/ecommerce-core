package service

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"gorm.io/gorm"
)

type promoService struct {
	repo domain.PromoRepository
	l    logger.Logger
	db   *gorm.DB // For CTE and checking hierarchy
}

func NewPromoService(repo domain.PromoRepository, l logger.Logger, db *gorm.DB) domain.PromoService {
	return &promoService{
		repo: repo,
		l:    l,
		db:   db,
	}
}

func (s *promoService) CreatePromo(ctx context.Context, p *domain.PromoCode) (*domain.PromoCode, error) {
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, p.ID)
}

func (s *promoService) GetPromoByID(ctx context.Context, id uuid.UUID) (*domain.PromoCode, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *promoService) GetPromoByCode(ctx context.Context, code string) (*domain.PromoCode, error) {
	return s.repo.GetByCode(ctx, code)
}

func (s *promoService) UpdatePromo(ctx context.Context, id uuid.UUID, p *domain.PromoCode) (*domain.PromoCode, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	p.ID = existing.ID
	p.CreatedAt = existing.CreatedAt

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, id)
}

func (s *promoService) DeletePromo(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *promoService) ListPromos(ctx context.Context, pgn pagination.Params) ([]domain.PromoCode, pagination.Metadata, error) {
	promos, total, err := s.repo.List(ctx, pgn.Page, pgn.Limit)
	if err != nil {
		return nil, pagination.Metadata{}, err
	}

	var data []domain.PromoCode
	for _, p := range promos {
		data = append(data, *p)
	}

	meta := pagination.CalculateMetadata(total, pgn.Page, pgn.Limit)
	return data, meta, nil
}

// CalculateCartDiscount and eligibility methods can be added here or in CartService
// We need a helper to check if a category is part of the promo's categories (including subcategories)
func (s *promoService) CheckCategoryHierarchy(ctx context.Context, targetCategoryID uuid.UUID, allowedCategoryIDs []uuid.UUID) (bool, error) {
	if len(allowedCategoryIDs) == 0 {
		return false, nil
	}
	
	// Check if target is directly in allowed
	for _, id := range allowedCategoryIDs {
		if id == targetCategoryID {
			return true, nil
		}
	}

	// Recursive CTE to find all subcategories of allowed categories
	query := `
		WITH RECURSIVE subcategories AS (
			SELECT id FROM category WHERE id IN ?
			UNION ALL
			SELECT c.id FROM category c
			INNER JOIN subcategories sc ON c.parent_id = sc.id
		)
		SELECT COUNT(1) FROM subcategories WHERE id = ?;
	`
	var count int
	if err := s.db.WithContext(ctx).Raw(query, allowedCategoryIDs, targetCategoryID).Scan(&count).Error; err != nil {
		return false, err
	}
	
	return count > 0, nil
}

func (s *promoService) ValidatePromoLimits(ctx context.Context, promoID uuid.UUID, userID *uuid.UUID, email, phone *string) error {
	p, err := s.repo.GetByID(ctx, promoID)
	if err != nil {
		return err
	}

	if !p.IsActive {
		return domain.ErrPromoCodeInactive
	}
	if p.EndsAt != nil && p.EndsAt.Before(time.Now()) {
		return domain.ErrPromoCodeExpired
	}
	if p.UsageLimit != nil && p.UsageCount >= *p.UsageLimit {
		return domain.ErrPromoCodeLimitExceeded
	}

	// Guest check is only effective if email or phone is provided
	if userID != nil || (email != nil && *email != "") || (phone != nil && *phone != "") {
		userUsages, err := s.repo.CheckUserUsage(ctx, promoID, userID, email, phone)
		if err != nil {
			return err
		}
		if p.UsageLimitPerUser != nil && userUsages >= *p.UsageLimitPerUser {
			return domain.ErrPromoCodeUserLimitExceeded
		}
	}

	return nil
}

func (s *promoService) CalculateCartDiscount(ctx context.Context, promoID uuid.UUID, items []domain.PromoItemInfo) (*domain.PromoCalculationResult, error) {
	p, err := s.repo.GetByID(ctx, promoID)
	if err != nil {
		return nil, err
	}

	var eligibleItems []domain.PromoItemInfo
	eligibleSubtotal := 0

	for _, item := range items {
		isEligible := true

		if len(p.ProductIDs) > 0 {
			match := false
			for _, pid := range p.ProductIDs {
				if pid == item.ProductID {
					match = true
					break
				}
			}
			if !match {
				isEligible = false
			}
		}

		if isEligible && len(p.BrandIDs) > 0 {
			match := false
			if item.BrandID != nil {
				for _, bid := range p.BrandIDs {
					if bid == *item.BrandID {
						match = true
						break
					}
				}
			}
			if !match {
				isEligible = false
			}
		}

		if isEligible && len(p.CategoryIDs) > 0 {
			match, err := s.CheckCategoryHierarchy(ctx, item.CategoryID, p.CategoryIDs)
			if err != nil {
				return nil, err
			}
			if !match {
				isEligible = false
			}
		}

		if isEligible {
			eligibleItems = append(eligibleItems, item)
			eligibleSubtotal += item.Price * item.Quantity
		}
	}

	if len(eligibleItems) == 0 {
		return nil, domain.ErrPromoCodeInvalidCart
	}

	if p.MinOrderSubtotal > 0 && eligibleSubtotal < p.MinOrderSubtotal {
		return nil, domain.ErrPromoCodeMinSubtotal
	}

	res := &domain.PromoCalculationResult{
		ItemDiscounts: make(map[uuid.UUID]int),
	}

	if p.DiscountType == domain.DiscountTypePercent {
		for _, item := range eligibleItems {
			// p.DiscountValue is in percent (e.g. 20 for 20%)
			// Discount amount per unit = Price * Percent / 100
			// Total for item = Discount amount per unit * Quantity
			// Need to compute carefully
			itemTotal := item.Price * item.Quantity
			discount := (itemTotal * p.DiscountValue) / 100
			res.ItemDiscounts[item.VariationID] = discount
			res.TotalDiscountAmount += discount
		}
	} else if p.DiscountType == domain.DiscountTypeFixed {
		// Distribute fixed discount across eligible items proportionally to their total price
		discountToDistribute := p.DiscountValue
		if discountToDistribute > eligibleSubtotal {
			discountToDistribute = eligibleSubtotal
		}

		distributed := 0
		for i, item := range eligibleItems {
			itemTotal := item.Price * item.Quantity
			
			var discount int
			if i == len(eligibleItems)-1 {
				// Last item takes the remainder
				discount = discountToDistribute - distributed
			} else {
				discount = int(math.Round(float64(discountToDistribute) * float64(itemTotal) / float64(eligibleSubtotal)))
			}
			
			// Protect against negative prices:
			if discount > itemTotal {
				discount = itemTotal
			}
			
			res.ItemDiscounts[item.VariationID] = discount
			distributed += discount
		}
		res.TotalDiscountAmount = distributed
	}

	return res, nil
}
