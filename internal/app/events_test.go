package app

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	comparisonDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/domain"
	wishlistDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
)

func TestWishlistLoginObserverMergesOnlyExistingGuestSession(t *testing.T) {
	service := &observerWishlistService{}
	observer := newWishlistLoginObserver(service)
	userID, sessionID := uuid.New(), uuid.New()

	if err := observer.OnUserLogin(context.Background(), userID, nil); err != nil {
		t.Fatalf("OnUserLogin(nil session) error = %v", err)
	}
	if service.calls != 0 {
		t.Fatalf("merge calls = %d, want 0", service.calls)
	}
	if err := observer.OnUserLogin(context.Background(), userID, &sessionID); err != nil {
		t.Fatalf("OnUserLogin() error = %v", err)
	}
	if service.calls != 1 || service.userID != userID || service.sessionID != sessionID {
		t.Fatalf("merge = (%d, %s, %s), want (1, %s, %s)", service.calls, service.userID, service.sessionID, userID, sessionID)
	}
}

func TestUserLoginObserverCallsEveryEnabledModule(t *testing.T) {
	wishlist := &observerWishlistService{}
	comparison := &observerComparisonService{}
	observer := newUserLoginObserver(newWishlistLoginObserver(wishlist), newComparisonLoginObserver(comparison))
	userID, sessionID := uuid.New(), uuid.New()

	if err := observer.OnUserLogin(context.Background(), userID, &sessionID); err != nil {
		t.Fatalf("OnUserLogin() error = %v", err)
	}
	if wishlist.calls != 1 || comparison.calls != 1 || comparison.userID != userID || comparison.sessionID != sessionID {
		t.Fatalf("observers not called correctly: wishlist=%d comparison=(%d, %s, %s)", wishlist.calls, comparison.calls, comparison.userID, comparison.sessionID)
	}
}

func TestUserLoginObserverLogsAndIgnoresModuleFailure(t *testing.T) {
	called := false
	observer := newUserLoginObserver(failingLoginObserver{}, callbackLoginObserver{called: &called})
	if err := observer.OnUserLogin(context.Background(), uuid.New(), uuidPointer(uuid.New())); err != nil {
		t.Fatalf("OnUserLogin() error = %v, want nil for best-effort observers", err)
	}
	if !called {
		t.Fatal("observer after a failed module was not called")
	}
}

type failingLoginObserver struct{}

func (failingLoginObserver) OnUserLogin(context.Context, uuid.UUID, *uuid.UUID) error {
	return errors.New("module database unavailable")
}

type callbackLoginObserver struct{ called *bool }

func (observer callbackLoginObserver) OnUserLogin(context.Context, uuid.UUID, *uuid.UUID) error {
	*observer.called = true
	return nil
}

func uuidPointer(value uuid.UUID) *uuid.UUID { return &value }

type observerWishlistService struct {
	calls     int
	userID    uuid.UUID
	sessionID uuid.UUID
}

func (service *observerWishlistService) List(context.Context, wishlistDomain.Owner) (*wishlistDomain.Wishlist, error) {
	return nil, nil
}
func (service *observerWishlistService) Add(context.Context, wishlistDomain.Owner, uuid.UUID) error {
	return nil
}
func (service *observerWishlistService) Remove(context.Context, wishlistDomain.Owner, uuid.UUID) error {
	return nil
}
func (service *observerWishlistService) MergeGuestWishlist(_ context.Context, userID, sessionID uuid.UUID) error {
	service.calls++
	service.userID, service.sessionID = userID, sessionID
	return nil
}

type observerComparisonService struct {
	calls     int
	userID    uuid.UUID
	sessionID uuid.UUID
}

func (*observerComparisonService) List(context.Context, comparisonDomain.Owner) (*comparisonDomain.Comparison, error) {
	return nil, nil
}
func (*observerComparisonService) Add(context.Context, comparisonDomain.Owner, uuid.UUID) error {
	return nil
}
func (*observerComparisonService) Remove(context.Context, comparisonDomain.Owner, uuid.UUID) error {
	return nil
}
func (service *observerComparisonService) MergeGuestComparison(_ context.Context, userID, sessionID uuid.UUID) error {
	service.calls++
	service.userID, service.sessionID = userID, sessionID
	return nil
}
