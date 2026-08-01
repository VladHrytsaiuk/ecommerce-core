// Package ukrposhta реалізує адаптер для майбутньої інтеграції з API Укрпошти.
package ukrposhta

import (
	"context"
	"errors"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"
)

// Client представляє HTTP-клієнт для API Укрпошти
type Client struct {
	l logger.Logger
}

// NewClient створює новий екземпляр клієнта
func NewClient(l logger.Logger) *Client {
	return &Client{l: l}
}

// ПРИМІТКА: Наступні методи є заглушками для реалізації інтерфейсу shipmentDomain.ShipmentProvider

func (c *Client) GetAreas(ctx context.Context) ([]domain.Area, error) {
	return nil, errors.New("ukrposhta integration not implemented yet")
}

func (c *Client) GetCities(ctx context.Context, areaRef string) ([]domain.City, error) {
	return nil, errors.New("ukrposhta integration not implemented yet")
}

func (c *Client) GetWarehouses(ctx context.Context, cityRef string, warehouseType string) ([]domain.Warehouse, error) {
	return nil, errors.New("ukrposhta integration not implemented yet")
}
