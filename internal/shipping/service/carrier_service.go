package service

import (
	"context"
	"fmt"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/integration/novaposhta"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shipping/domain"
	shipmentDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"
)

type carrierService struct {
	npClient     *novaposhta.Client
	shipmentSvc  shipmentDomain.ShipmentService
	cfg          *config.Config
	l            logger.Logger
}

// NewCarrierService створює сервіс для роботи з перевізниками
func NewCarrierService(npClient *novaposhta.Client, shipmentSvc shipmentDomain.ShipmentService, cfg *config.Config, l logger.Logger) domain.CarrierService {
	return &carrierService{
		npClient:    npClient,
		shipmentSvc: shipmentSvc,
		cfg:         cfg,
		l:           l,
	}
}

// CreateShipment створює відправлення через відповідного перевізника
func (s *carrierService) CreateShipment(ctx context.Context, req domain.CreateShipmentRequest) (*domain.ShipmentResult, error) {
	switch req.Provider {
	case "novaposhta":
		return s.createNovaPoshtaShipment(ctx, req)
	default:
		s.l.Warnw("Unknown shipping provider for TTN creation", "provider", req.Provider)
		return nil, shipmentDomain.ErrUnknownProvider
	}
}

// createNovaPoshtaShipment створює ТТН через API Нової Пошти
func (s *carrierService) createNovaPoshtaShipment(ctx context.Context, req domain.CreateShipmentRequest) (*domain.ShipmentResult, error) {
	if s.cfg.NPSenderRef == "" || s.cfg.NPContactSenderRef == "" {
		s.l.Error("Nova Poshta sender configuration is missing, cannot create TTN")
		return nil, fmt.Errorf("%w: NP sender not configured", domain.ErrShipmentCreationFailed)
	}

	// Конвертуємо копійки в гривні для оголошеної вартості
	costUAH := fmt.Sprintf("%.2f", float64(req.DeclaredValue)/100)

	// Логіка безкоштовної доставки
	payerType := "Recipient"
	paymentMethod := "Cash"
	
	threshold, err := s.shipmentSvc.GetFreeShippingThreshold(ctx)
	if err != nil {
		s.l.Warnw("Failed to get free shipping threshold, defaulting to paid delivery", "error", err)
	} else if threshold > 0 && req.DeclaredValue >= threshold {
		payerType = "Sender"
		paymentMethod = "NonCash" // Магазин платить зі свого рахунку по договору
		s.l.Infow("Free shipping applied", "order_value", req.DeclaredValue, "threshold", threshold)
	}

	docReq := novaposhta.CreateDocumentRequest{
		SenderRef:           s.cfg.NPSenderRef,
		SenderAddressRef:    s.cfg.NPSenderAddressRef,
		ContactSenderRef:    s.cfg.NPContactSenderRef,
		SenderPhone:         s.cfg.NPSenderPhone,
		RecipientName:       req.RecipientName,
		RecipientPhone:      req.RecipientPhone,
		CityRecipientRef:    req.CityRef,
		RecipientAddressRef: req.WarehouseRef,
		Weight:              req.Weight,
		Description:         req.Description,
		PayerType:           payerType,
		PaymentMethod:       paymentMethod,
		ServiceType:         "WarehouseWarehouse",
		SeatsAmount:         "1",
		Cost:                costUAH,
	}

	if docReq.Weight == "" {
		docReq.Weight = "0.5" // default weight
	}

	docResp, err := s.npClient.CreateInternetDocument(ctx, docReq)
	if err != nil {
		s.l.Errorw("Failed to create Nova Poshta TTN",
			"error", err,
			"recipient_phone", req.RecipientPhone,
			"city_ref", req.CityRef,
		)
		return nil, fmt.Errorf("%w: %v", domain.ErrShipmentCreationFailed, err)
	}

	rawJSON := string(docResp.RawJSON)

	s.l.Infow("TTN created via Nova Poshta",
		"tracking_number", docResp.IntDocNumber,
		"ref", docResp.Ref,
	)

	return &domain.ShipmentResult{
		Provider:       "novaposhta",
		TrackingNumber: docResp.IntDocNumber,
		Ref:            docResp.Ref,
		RawResponse:    rawJSON,
	}, nil
}

// TrackShipment запитує статус відправлення у перевізника
func (s *carrierService) TrackShipment(ctx context.Context, trackingNumber, phone string) (*domain.TrackingResult, error) {
	// Зараз підтримується лише novaposhta, але логіку можна розширити перевіркою provider замовлення.
	// Для MVP вважаємо, що трекінг завжди НП.

	resp, err := s.npClient.GetDocumentTracking(ctx, trackingNumber, phone)
	if err != nil {
		s.l.Errorw("Failed to track NP shipment", "error", err, "tracking_number", trackingNumber)
		return nil, err
	}

	unifiedStatus, isDelivered := MapNPStatus(resp.StatusCode)

	rawResponseJSON := fmt.Sprintf(`{"StatusCode":"%s","Status":"%s","ActualDeliveryDate":"%s"}`, resp.StatusCode, resp.Status, resp.ActualDeliveryDate)

	return &domain.TrackingResult{
		TrackingNumber: trackingNumber,
		Status:         unifiedStatus,
		RawStatus:      resp.Status,
		IsDelivered:    isDelivered,
		RawResponse:    rawResponseJSON,
	}, nil
}
