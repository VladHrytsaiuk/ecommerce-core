//go:build legacy && ignore
// +build legacy,ignore

package domain

import "fmt"

// AllowedTransitions визначає допустимі переходи між статусами замовлення.
// Використовується ЛИШЕ на рівні сервісів (не repository).
var AllowedTransitions = map[int][]int{
	StatusPendingPayment: {StatusPaid, StatusCancelled},
	StatusPaid:           {StatusProcessing},
	StatusProcessing:     {StatusShipped, StatusCancelled},
	StatusShipped:        {StatusDelivered},
	// Delivered, Cancelled, Refunded — фінальні статуси, переходів немає
}

// ValidateTransition перевіряє, чи дозволений перехід з from у to.
func ValidateTransition(from, to int) error {
	allowed, ok := AllowedTransitions[from]
	if !ok {
		return fmt.Errorf("%w: no transitions from status %d", ErrInvalidStatus, from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("%w: %d → %d", ErrInvalidStatus, from, to)
}
