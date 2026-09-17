package orders

import (
	"context"
	consent "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ActivityReader struct{ db *gorm.DB }

func NewActivityReader(db *gorm.DB) *ActivityReader { return &ActivityReader{db} }
func (r *ActivityReader) HasActiveOrders(c context.Context, id uuid.UUID) (bool, error) {
	var n int64
	e := r.db.WithContext(c).Table("orders").Where("customer_id=? AND status NOT IN ?", id, []string{"cancelled", "refunded", "closed"}).Count(&n).Error
	return n > 0, e
}

var _ consent.OrderActivityReader = (*ActivityReader)(nil)
