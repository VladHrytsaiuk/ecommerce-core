package app

import (
	"context"

	"github.com/google/uuid"

	comparisonDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/domain"
	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	wishlistDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
)

type userLoginObservers []identityDomain.UserLoginObserver

func newUserLoginObserver(observers ...identityDomain.UserLoginObserver) identityDomain.UserLoginObserver {
	active := make(userLoginObservers, 0, len(observers))
	for _, observer := range observers {
		if observer != nil {
			active = append(active, observer)
		}
	}
	if len(active) == 0 {
		return nil
	}
	return active
}

func (observers userLoginObservers) OnUserLogin(ctx context.Context, userID uuid.UUID, sessionID *uuid.UUID) error {
	for _, observer := range observers {
		if err := observer.OnUserLogin(ctx, userID, sessionID); err != nil {
			// Engagement data is optional. A transient module/database failure must
			// not prevent the authenticated identity flow from issuing its JWT.
			// The next successful login retries the idempotent merge.
			if logger.Log != nil {
				logger.Log.Errorw("best-effort engagement merge failed after login", "error", err)
			}
		}
	}
	return nil
}

// wishlistLoginObserver is assembled only when Wishlist is enabled. Identity
// owns the event port; the Composition Root selects this optional subscriber.
type wishlistLoginObserver struct{ service wishlistDomain.Service }

func newWishlistLoginObserver(service wishlistDomain.Service) identityDomain.UserLoginObserver {
	if service == nil {
		return nil
	}
	return wishlistLoginObserver{service: service}
}

func (observer wishlistLoginObserver) OnUserLogin(ctx context.Context, userID uuid.UUID, sessionID *uuid.UUID) error {
	if observer.service == nil || sessionID == nil || *sessionID == uuid.Nil {
		return nil
	}
	return observer.service.MergeGuestWishlist(ctx, userID, *sessionID)
}

type comparisonLoginObserver struct{ service comparisonDomain.Service }

func newComparisonLoginObserver(service comparisonDomain.Service) identityDomain.UserLoginObserver {
	if service == nil {
		return nil
	}
	return comparisonLoginObserver{service: service}
}

func (observer comparisonLoginObserver) OnUserLogin(ctx context.Context, userID uuid.UUID, sessionID *uuid.UUID) error {
	if sessionID == nil || *sessionID == uuid.Nil {
		return nil
	}
	return observer.service.MergeGuestComparison(ctx, userID, *sessionID)
}
