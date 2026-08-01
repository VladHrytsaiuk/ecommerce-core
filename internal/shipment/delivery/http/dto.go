package http

import "github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"

// AreaResponse — DTO області для відповіді фронтенду
type AreaResponse struct {
	Ref         string `json:"ref"`
	Description string `json:"description"`
}

// CityResponse — DTO міста для відповіді фронтенду
type CityResponse struct {
	Ref         string `json:"ref"`
	Description string `json:"description"`
	AreaRef     string `json:"area_ref"`
}

// WarehouseResponse — DTO відділення/поштомату для відповіді фронтенду
type WarehouseResponse struct {
	Ref             string `json:"ref"`
	Description     string `json:"description"`
	ShortAddress    string `json:"short_address"`
	Number          string `json:"number"`
	TypeOfWarehouse string `json:"type_of_warehouse"`
}

// ShippingConfigResponse — DTO для налаштувань доставки
type ShippingConfigResponse struct {
	FreeShippingThreshold int `json:"free_shipping_threshold"` // в копійках
	MinOrderAmount        int `json:"min_order_amount"`        // в копійках
}

// UpdateShippingRuleRequest — DTO для оновлення правил доставки
type UpdateShippingRuleRequest struct {
	Provider       string `json:"provider" binding:"required"`
	MinOrderAmount int    `json:"min_order_amount" binding:"min=0"`
}

// ErrorResponse — стандартна структура помилки
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func mapAreasToResponse(areas []domain.Area) []AreaResponse {
	res := make([]AreaResponse, len(areas))
	for i, a := range areas {
		res[i] = AreaResponse{
			Ref:         a.Ref,
			Description: a.Description,
		}
	}
	return res
}

func mapCitiesToResponse(cities []domain.City) []CityResponse {
	res := make([]CityResponse, len(cities))
	for i, c := range cities {
		res[i] = CityResponse{
			Ref:         c.Ref,
			Description: c.Description,
			AreaRef:     c.AreaRef,
		}
	}
	return res
}

func mapWarehousesToResponse(warehouses []domain.Warehouse) []WarehouseResponse {
	res := make([]WarehouseResponse, len(warehouses))
	for i, w := range warehouses {
		res[i] = WarehouseResponse{
			Ref:             w.Ref,
			Description:     w.Description,
			ShortAddress:    w.ShortAddress,
			Number:          w.Number,
			TypeOfWarehouse: w.TypeOfWarehouse,
		}
	}
	return res
}
