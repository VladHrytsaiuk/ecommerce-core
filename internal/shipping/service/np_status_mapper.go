package service

// Unified shipment statuses
const (
	ShipmentStatusShipped   = "shipped"
	ShipmentStatusDelivered = "delivered"
	ShipmentStatusReturned  = "returned"
	ShipmentStatusUnknown   = "unknown"
)

// MapNPStatus приймає StatusCode від Нової Пошти і повертає уніфікований статус
// та прапорець isDelivered.
// Документація НП: https://developers.novaposhta.ua/view/model/a99d2f28-8512-11ec-8ced-005056b2dbe1/method/a9ae7bc9-8512-11ec-8ced-005056b2dbe1
func MapNPStatus(statusCode string) (status string, isDelivered bool) {
	switch statusCode {
	// Доставлено
	case "9", "10", "11":
		return ShipmentStatusDelivered, true
	
	// В дорозі
	case "1", "2", "3", "4", "41", "5", "6", "7", "8", "101":
		return ShipmentStatusShipped, false
	
	// Відмова / Повернення
	case "102", "103", "104", "108", "111", "112":
		return ShipmentStatusReturned, false
	
	default:
		return ShipmentStatusUnknown, false
	}
}
