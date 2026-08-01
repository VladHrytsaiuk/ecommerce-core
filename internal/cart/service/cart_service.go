package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	discountDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
	shipmentDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"
)

type cartService struct {
	repo     domain.CartRepository
	shipment shipmentDomain.ShipmentService
	promo    discountDomain.PromoService
	l        logger.Logger
}

// NewCartService створює новий інстанс сервісу
func NewCartService(repo domain.CartRepository, shipment shipmentDomain.ShipmentService, promo discountDomain.PromoService, l logger.Logger) domain.CartService {
	return &cartService{repo: repo, shipment: shipment, promo: promo, l: l}
}

// GetFullCart повертає повну структуру кошика, пов'язані варіації та інформацію про доставку
func (s *cartService) GetFullCart(ctx context.Context, userID *uuid.UUID, sessionID *string, lang string) (*domain.Cart, []productDomain.ProductVariation, *domain.ShippingSummary, *discountDomain.PromoCalculationResult, *string, error) {
	if userID == nil && (sessionID == nil || *sessionID == "") {
		shipping := s.calculateShipping(ctx, 0)
		return &domain.Cart{Items: []domain.CartItem{}}, []productDomain.ProductVariation{}, shipping, nil, nil, nil
	}

	var cart *domain.Cart
	var variations []productDomain.ProductVariation
	var err error

	if userID != nil {
		cart, variations, err = s.repo.GetByUserID(ctx, *userID, lang)
	} else {
		cart, variations, err = s.repo.GetBySessionID(ctx, *sessionID, lang)
	}
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	// Рахуємо загальну суму
	totalPrice := 0
	variationMap := make(map[uuid.UUID]productDomain.ProductVariation, len(variations))
	if cart != nil {
		for _, v := range variations {
			variationMap[v.ID] = v
		}
		for _, item := range cart.Items {
			if v, ok := variationMap[item.VariationID]; ok {
				totalPrice += v.Price * item.Quantity
			}
		}
	}

	var promoResult *discountDomain.PromoCalculationResult
	var promoCodeStr *string

	// Пробуємо застосувати промокод, якщо він є
	if cart != nil && cart.PromoCodeID != nil {
		p, err := s.promo.GetPromoByID(ctx, *cart.PromoCodeID)
		if err == nil && p != nil {
			err = s.promo.ValidatePromoLimits(ctx, p.ID, userID, nil, nil)
			if err != nil {
				_ = s.repo.UpdatePromoCode(ctx, userID, sessionID, nil)
				cart.PromoCodeID = nil
			} else {
				var promoItems []discountDomain.PromoItemInfo
				for _, item := range cart.Items {
					if v, ok := variationMap[item.VariationID]; ok {
						var brandID *uuid.UUID
						if v.Product.BrandID != uuid.Nil {
							brandID = &v.Product.BrandID
						}
						promoItems = append(promoItems, discountDomain.PromoItemInfo{
							VariationID: v.ID,
							ProductID:   v.ProductID,
							CategoryID:  v.Product.CategoryID,
							BrandID:     brandID,
							Price:       v.Price,
							Quantity:    item.Quantity,
						})
					}
				}
				promoResult, err = s.promo.CalculateCartDiscount(ctx, p.ID, promoItems)
				if err != nil {
					_ = s.repo.UpdatePromoCode(ctx, userID, sessionID, nil)
					cart.PromoCodeID = nil
					promoResult = nil
				} else {
					promoCodeStr = &p.Code
					totalPrice -= promoResult.TotalDiscountAmount
				}
			}
		} else {
			_ = s.repo.UpdatePromoCode(ctx, userID, sessionID, nil)
			cart.PromoCodeID = nil
		}
	}

	shipping := s.calculateShipping(ctx, totalPrice)
	return cart, variations, shipping, promoResult, promoCodeStr, nil
}

// calculateShipping обчислює інформацію про безкоштовну доставку
func (s *cartService) calculateShipping(ctx context.Context, totalPrice int) *domain.ShippingSummary {
	minOrder, minOrderErr := s.shipment.GetMinimumOrderAmount(ctx)
	if minOrderErr != nil {
		s.l.Warnw("Failed to get minimum order amount, defaulting to 0", "error", minOrderErr)
		minOrder = 0
	}

	threshold, err := s.shipment.GetFreeShippingThreshold(ctx)
	if err != nil {
		s.l.Errorw("Failed to get shipping threshold, defaulting to no free shipping info", "error", err)
		return &domain.ShippingSummary{
			IsFreeShipping:  false,
			Threshold:       0,
			RemainingAmount: 0,
			MinOrderAmount:  minOrder,
		}
	}

	if threshold == 0 {
		// Немає правила — не показуємо інфо про безкоштовну доставку
		return &domain.ShippingSummary{
			IsFreeShipping:  true,
			Threshold:       0,
			RemainingAmount: 0,
			MinOrderAmount:  minOrder,
		}
	}

	isFree := totalPrice >= threshold
	remaining := 0
	if !isFree {
		remaining = threshold - totalPrice
	}

	return &domain.ShippingSummary{
		IsFreeShipping:  isFree,
		Threshold:       threshold,
		RemainingAmount: remaining,
		MinOrderAmount:  minOrder,
	}
}

// AddItem додає варіацію до кошика з валідацією
func (s *cartService) AddItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error {
	if userID == nil && (sessionID == nil || *sessionID == "") {
		return domain.ErrNoIdentifier
	}

	if quantity <= 0 {
		return domain.ErrInvalidQuantity
	}

	// Перевіряємо чи існує варіація
	exists, err := s.repo.VariationExists(ctx, variationID)
	if err != nil {
		return err
	}
	if !exists {
		return domain.ErrVariationNotFound
	}

	return s.repo.AddItem(ctx, userID, sessionID, variationID, quantity)
}

// UpdateQuantity змінює кількість товару в кошику
func (s *cartService) UpdateQuantity(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error {
	if userID == nil && (sessionID == nil || *sessionID == "") {
		return domain.ErrNoIdentifier
	}

	if quantity <= 0 {
		return domain.ErrInvalidQuantity
	}

	return s.repo.UpdateQuantity(ctx, userID, sessionID, variationID, quantity)
}

// RemoveItem видаляє варіацію з кошика
func (s *cartService) RemoveItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error {
	if userID == nil && (sessionID == nil || *sessionID == "") {
		return domain.ErrNoIdentifier
	}

	return s.repo.RemoveItem(ctx, userID, sessionID, variationID)
}

// SyncSession переносить анонімний кошик на авторизованого юзера
func (s *cartService) SyncSession(ctx context.Context, sessionID string, userID uuid.UUID) error {
	if sessionID == "" {
		return domain.ErrNoIdentifier
	}
	s.l.Infow("Starting cart sync", "session_id", sessionID, "user_id", userID)
	return s.repo.SyncSessionToUser(ctx, sessionID, userID)
}

func (s *cartService) ApplyPromoCode(ctx context.Context, userID *uuid.UUID, sessionID *string, code string, lang string) error {
	p, err := s.promo.GetPromoByCode(ctx, code)
	if err != nil {
		return err
	}

	var cart *domain.Cart
	var variations []productDomain.ProductVariation

	if userID != nil {
		cart, variations, err = s.repo.GetByUserID(ctx, *userID, lang)
	} else {
		cart, variations, err = s.repo.GetBySessionID(ctx, *sessionID, lang)
	}
	if err != nil {
		return err
	}

	if cart == nil || len(cart.Items) == 0 {
		return domain.ErrCartNotFound
	}

	if err := s.promo.ValidatePromoLimits(ctx, p.ID, userID, nil, nil); err != nil {
		return err
	}

	variationMap := make(map[uuid.UUID]productDomain.ProductVariation)
	for _, v := range variations {
		variationMap[v.ID] = v
	}

	var promoItems []discountDomain.PromoItemInfo
	for _, item := range cart.Items {
		if v, ok := variationMap[item.VariationID]; ok {
			var brandID *uuid.UUID
			if v.Product.BrandID != uuid.Nil {
				brandID = &v.Product.BrandID
			}
			promoItems = append(promoItems, discountDomain.PromoItemInfo{
				VariationID: v.ID,
				ProductID:   v.ProductID,
				CategoryID:  v.Product.CategoryID,
				BrandID:     brandID,
				Price:       v.Price,
				Quantity:    item.Quantity,
			})
		}
	}

	_, err = s.promo.CalculateCartDiscount(ctx, p.ID, promoItems)
	if err != nil {
		return err
	}

	return s.repo.UpdatePromoCode(ctx, userID, sessionID, &p.ID)
}

func (s *cartService) RemovePromoCode(ctx context.Context, userID *uuid.UUID, sessionID *string) error {
	return s.repo.UpdatePromoCode(ctx, userID, sessionID, nil)
}
