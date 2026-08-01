// Package domain містить доменні моделі та інтерфейси модуля доставки.
package domain

import (
	"context"
	"errors"
	"time"
)

// ErrUnknownProvider повертається, коли вказано невідомого провайдера доставки.
var ErrUnknownProvider = errors.New("unknown shipping provider")

// ErrProviderUnavailable повертається при тимчасових проблемах зі зв'язком з API стороннього сервісу.
var ErrProviderUnavailable = errors.New("shipping provider service is temporarily unavailable")

// ErrExternalAPI повертається при логічних помилках стороннього API (напр. невірні параметри, помилка авторизації).
var ErrExternalAPI = errors.New("external shipping api returned error")

// Area представляє область (регіон) від служби доставки
type Area struct {
	Ref         string `json:"ref"`
	Description string `json:"description"`
}

// City представляє місто від служби доставки
type City struct {
	Ref         string `json:"ref"`
	Description string `json:"description"`
	AreaRef     string `json:"area_ref"`
}

// Warehouse представляє відділення або поштомат від служби доставки
type Warehouse struct {
	Ref             string `json:"ref"`
	Description     string `json:"description"`
	ShortAddress    string `json:"short_address"`
	Number          string `json:"number"`
	TypeOfWarehouse string `json:"type_of_warehouse"`
}

// ShippingRule правило безкоштовної доставки
// provider = "all" — глобальне правило для всіх провайдерів
// provider = "novaposhta" / "ukrposhta" — специфічне правило для конкретного провайдера (на майбутнє)
type ShippingRule struct {
	ID             int       `gorm:"primaryKey" json:"id"`
	Provider       string    `gorm:"type:varchar(50);not null;default:'all'" json:"provider"`
	MinOrderAmount int       `gorm:"not null;default:0" json:"min_order_amount"` // в копійках
	IsActive       bool      `gorm:"not null;default:true" json:"is_active"`
	CreatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (ShippingRule) TableName() string {
	return "shipping_rule"
}

// ShippingRuleRepository контракт для роботи з правилами доставки в БД
type ShippingRuleRepository interface {
	// GetActiveRules повертає всі активні правила безкоштовної доставки
	GetActiveRules(ctx context.Context) ([]ShippingRule, error)
	// UpdateShippingRule оновлює або створює правило для провайдера
	UpdateShippingRule(ctx context.Context, provider string, minOrderAmount int) error
}

// ShipmentProvider визначає контракт для роботи з API конкретної служби доставки (Нова Пошта, Укрпошта тощо).
type ShipmentProvider interface {
	GetAreas(ctx context.Context) ([]Area, error)
	GetCities(ctx context.Context, areaRef string) ([]City, error)
	GetWarehouses(ctx context.Context, cityRef string, warehouseType string) ([]Warehouse, error)
}

// ShipmentService визначає контракт для бізнес-логіки модуля доставки з підтримкою кількох провайдерів.
type ShipmentService interface {
	GetAreas(ctx context.Context, provider string) ([]Area, error)
	GetCities(ctx context.Context, provider string, areaRef string) ([]City, error)
	GetWarehouses(ctx context.Context, provider string, cityRef string, warehouseType string) ([]Warehouse, error)
	// GetFreeShippingThreshold повертає мінімальну суму для безкоштовної доставки (глобальне правило)
	GetFreeShippingThreshold(ctx context.Context) (int, error)
	// GetMinimumOrderAmount повертає мінімальну суму замовлення (глобальне правило)
	GetMinimumOrderAmount(ctx context.Context) (int, error)
	// UpdateShippingRule оновлює або створює правило доставки
	UpdateShippingRule(ctx context.Context, provider string, minOrderAmount int) error
}
