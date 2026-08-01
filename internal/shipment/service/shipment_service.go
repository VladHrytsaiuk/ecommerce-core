package service

import (
	"context"
	"fmt"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"
)

type shipmentService struct {
	providers map[string]domain.ShipmentProvider
	ruleRepo  domain.ShippingRuleRepository
	l         logger.Logger
}

// NewShipmentService створює новий інстанс сервісу доставки з підтримкою кількох провайдерів.
// providers — це мапа, де ключ — назва провайдера (наприклад, "novaposhta", "ukrposhta").
func NewShipmentService(providers map[string]domain.ShipmentProvider, ruleRepo domain.ShippingRuleRepository, l logger.Logger) domain.ShipmentService {
	return &shipmentService{providers: providers, ruleRepo: ruleRepo, l: l}
}

// getProvider повертає провайдера за назвою або помилку, якщо провайдер невідомий.
func (s *shipmentService) getProvider(name string) (domain.ShipmentProvider, error) {
	p, ok := s.providers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", domain.ErrUnknownProvider, name)
	}
	return p, nil
}

// GetAreas повертає список областей від вказаного провайдера
func (s *shipmentService) GetAreas(ctx context.Context, provider string) ([]domain.Area, error) {
	p, err := s.getProvider(provider)
	if err != nil {
		return nil, err
	}

	areas, err := p.GetAreas(ctx)
	if err != nil {
		s.l.Errorw("ShipmentService: failed to get areas", "provider", provider, "err", err)
		return nil, err
	}
	return areas, nil
}

// GetCities повертає список міст для вказаної області від вказаного провайдера
func (s *shipmentService) GetCities(ctx context.Context, provider string, areaRef string) ([]domain.City, error) {
	p, err := s.getProvider(provider)
	if err != nil {
		return nil, err
	}

	cities, err := p.GetCities(ctx, areaRef)
	if err != nil {
		s.l.Errorw("ShipmentService: failed to get cities", "provider", provider, "area_ref", areaRef, "err", err)
		return nil, err
	}
	return cities, nil
}

// GetWarehouses повертає список відділень/поштоматів для вказаного міста від вказаного провайдера
func (s *shipmentService) GetWarehouses(ctx context.Context, provider string, cityRef string, warehouseType string) ([]domain.Warehouse, error) {
	p, err := s.getProvider(provider)
	if err != nil {
		return nil, err
	}

	warehouses, err := p.GetWarehouses(ctx, cityRef, warehouseType)
	if err != nil {
		s.l.Errorw("ShipmentService: failed to get warehouses", "provider", provider, "city_ref", cityRef, "type", warehouseType, "err", err)
		return nil, err
	}
	return warehouses, nil
}

// GetFreeShippingThreshold повертає мінімальну суму для безкоштовної доставки.
// Спершу шукає глобальне правило (provider = 'all'), якщо не знайдено — повертає 0 (завжди безкоштовно).
func (s *shipmentService) GetFreeShippingThreshold(ctx context.Context) (int, error) {
	rules, err := s.ruleRepo.GetActiveRules(ctx)
	if err != nil {
		return 0, err
	}

	// Шукаємо глобальне правило
	for _, rule := range rules {
		if rule.Provider == "all" {
			return rule.MinOrderAmount, nil
		}
	}

	// Якщо немає правил — безкоштовна доставка не встановлена
	return 0, nil
}

// GetMinimumOrderAmount повертає мінімальну суму замовлення.
// Шукає правило (provider = 'min_order'), якщо не знайдено — повертає 0.
func (s *shipmentService) GetMinimumOrderAmount(ctx context.Context) (int, error) {
	rules, err := s.ruleRepo.GetActiveRules(ctx)
	if err != nil {
		return 0, err
	}

	// Шукаємо глобальне правило мінімального замовлення
	for _, rule := range rules {
		if rule.Provider == "min_order" {
			return rule.MinOrderAmount, nil
		}
	}

	return 0, nil
}

// UpdateShippingRule оновлює або створює правило доставки
func (s *shipmentService) UpdateShippingRule(ctx context.Context, provider string, minOrderAmount int) error {
	return s.ruleRepo.UpdateShippingRule(ctx, provider, minOrderAmount)
}
