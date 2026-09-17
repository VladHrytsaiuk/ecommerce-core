package application

import (
	"context"
	availability "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/domain"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestSubscribeNormalizesAndUsesProfileFallback(t *testing.T) {
	repo := &subscriptionRepo{}
	s := NewService(repo, emailReader("buyer@example.com"))
	v := uuid.New()
	out, created, err := s.Subscribe(context.Background(), v, "", uuid.New())
	if err != nil || !created || out.Email != "buyer@example.com" {
		t.Fatalf("subscribe=%+v %v %v", out, created, err)
	}
}

type subscriptionRepo struct{}

func (*subscriptionRepo) Create(_ context.Context, s availability.Subscription) (*availability.Subscription, bool, error) {
	s.ID = uuid.New()
	return &s, true, nil
}
func (*subscriptionRepo) MarkPendingNotified(context.Context, uuid.UUID, time.Time) ([]availability.Subscription, error) {
	return nil, nil
}

type emailReader string

func (e emailReader) EmailForCustomer(context.Context, uuid.UUID) (string, error) {
	return string(e), nil
}
