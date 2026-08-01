// Package domain містить доменні моделі та інтерфейси модуля створення відправлень.
package domain

import (
	"context"
	"errors"
)

var (
	ErrShipmentCreationFailed = errors.New("shipment creation failed")
)

// CreateShipmentRequest параметри для створення відправлення
type CreateShipmentRequest struct {
	Provider       string
	RecipientName  string
	RecipientPhone string
	CityRef        string
	WarehouseRef   string
	Weight         string
	Description    string
	DeclaredValue  int // копійки
}

// ShipmentResult результат створення відправлення від перевізника
type ShipmentResult struct {
	Provider       string
	TrackingNumber string
	Ref            string
	RawResponse    string
}

// TrackingResult результат трекінгу відправлення
type TrackingResult struct {
	TrackingNumber string
	Status         string // 'delivered', 'shipped', 'returned', 'unknown'
	RawStatus      string
	IsDelivered    bool
	RawResponse    string
}

// CarrierService контракт для створення відправлень через перевізників
type CarrierService interface {
	CreateShipment(ctx context.Context, req CreateShipmentRequest) (*ShipmentResult, error)
	TrackShipment(ctx context.Context, trackingNumber, phone string) (*TrackingResult, error)
}
