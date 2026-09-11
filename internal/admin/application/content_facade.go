package application

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	badgesDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	reviewsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
	seoDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/seo/domain"
)

const (
	PermissionBadgesWrite  = "badges:write"
	PermissionReviewsWrite = "reviews:write"
	PermissionSEOWrite     = "seo:write"
)

// ContentAdminFacade is the audited mutation boundary for the three catalog
// enrichment modules: badges, review moderation and SEO metadata.
//
// They share one facade because they share one shape — a small permissioned
// change to presentation data, with no cross-module workflow — and because
// each is independently optional. A deployment that enables only badges gets a
// facade whose other two services are nil and whose routes are not registered.
//
// Every mutation writes its audit event in the same transaction as the change.
// That is the condition router.go states for exposing a permission-protected
// admin route at all: a moderator hiding a review or rewriting a page title
// without a forensic record is exactly the bypass it refuses to allow.
type ContentAdminFacade struct {
	authorizer adminDomain.Authorizer
	tx         TransactionManager
	publisher  events.TransactionalEventPublisher
	badges     badgesDomain.Service
	reviews    reviewsDomain.Service
	seo        seoDomain.Service
}

func NewContentAdminFacade(authorizer adminDomain.Authorizer, tx TransactionManager, publisher events.TransactionalEventPublisher) (*ContentAdminFacade, error) {
	if authorizer == nil || tx == nil || publisher == nil {
		return nil, fmt.Errorf("content admin facade is not configured")
	}
	return &ContentAdminFacade{authorizer: authorizer, tx: tx, publisher: publisher}, nil
}

// WithBadges, WithReviews and WithSEO attach whichever modules Bootstrap
// enabled. An absent service leaves its routes unregistered rather than
// failing at request time.
func (f *ContentAdminFacade) WithBadges(service badgesDomain.Service) *ContentAdminFacade {
	f.badges = service
	return f
}

func (f *ContentAdminFacade) WithReviews(service reviewsDomain.Service) *ContentAdminFacade {
	f.reviews = service
	return f
}

func (f *ContentAdminFacade) WithSEO(service seoDomain.Service) *ContentAdminFacade {
	f.seo = service
	return f
}

func (f *ContentAdminFacade) HasBadges() bool  { return f != nil && f.badges != nil }
func (f *ContentAdminFacade) HasReviews() bool { return f != nil && f.reviews != nil }
func (f *ContentAdminFacade) HasSEO() bool     { return f != nil && f.seo != nil }

// ---------- badges ----------

func (f *ContentAdminFacade) CreateBadge(ctx context.Context, cmd CatalogCommand, command badgesDomain.CreateCommand) (*badgesDomain.Badge, error) {
	if err := f.authorize(ctx, cmd.ActorUserID, PermissionBadgesWrite, f.badges == nil, "badges"); err != nil {
		return nil, err
	}
	var created *badgesDomain.Badge
	err := f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		badge, err := f.badges.Create(txCtx, command)
		if err != nil {
			return err
		}
		created = badge
		return f.audit(txCtx, cmd, "badges.badge.create", "badge", badge.ID, nil, badge)
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (f *ContentAdminFacade) UpdateBadge(ctx context.Context, cmd CatalogCommand, badgeID uuid.UUID, command badgesDomain.UpdateCommand) (*badgesDomain.Badge, error) {
	if err := f.authorize(ctx, cmd.ActorUserID, PermissionBadgesWrite, f.badges == nil, "badges"); err != nil {
		return nil, err
	}
	if badgeID == uuid.Nil {
		return nil, fmt.Errorf("badge ID is required")
	}
	var updated *badgesDomain.Badge
	err := f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		// The prior state is read inside the transaction so the audit entry
		// records what was actually replaced, not a value that may have moved.
		previous, err := f.badges.Get(txCtx, badgeID)
		if err != nil {
			return err
		}
		badge, err := f.badges.Update(txCtx, badgeID, command)
		if err != nil {
			return err
		}
		updated = badge
		return f.audit(txCtx, cmd, "badges.badge.update", "badge", badgeID, previous, badge)
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (f *ContentAdminFacade) DeleteBadge(ctx context.Context, cmd CatalogCommand, badgeID uuid.UUID) error {
	if err := f.authorize(ctx, cmd.ActorUserID, PermissionBadgesWrite, f.badges == nil, "badges"); err != nil {
		return err
	}
	if badgeID == uuid.Nil {
		return fmt.Errorf("badge ID is required")
	}
	return f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		previous, err := f.badges.Get(txCtx, badgeID)
		if err != nil {
			return err
		}
		if err := f.badges.Delete(txCtx, badgeID); err != nil {
			return err
		}
		return f.audit(txCtx, cmd, "badges.badge.delete", "badge", badgeID, previous, nil)
	})
}

func (f *ContentAdminFacade) AssignBadge(ctx context.Context, cmd CatalogCommand, badgeID, productID uuid.UUID) error {
	return f.badgeAssignment(ctx, cmd, badgeID, productID, true)
}

func (f *ContentAdminFacade) RemoveBadge(ctx context.Context, cmd CatalogCommand, badgeID, productID uuid.UUID) error {
	return f.badgeAssignment(ctx, cmd, badgeID, productID, false)
}

func (f *ContentAdminFacade) badgeAssignment(ctx context.Context, cmd CatalogCommand, badgeID, productID uuid.UUID, assign bool) error {
	if err := f.authorize(ctx, cmd.ActorUserID, PermissionBadgesWrite, f.badges == nil, "badges"); err != nil {
		return err
	}
	if badgeID == uuid.Nil || productID == uuid.Nil {
		return fmt.Errorf("badge and product IDs are required")
	}
	action, payload := "badges.badge.unassign", map[string]any{"badge_id": badgeID, "product_id": productID}
	if assign {
		action = "badges.badge.assign"
	}
	return f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		if assign {
			err = f.badges.AssignProduct(txCtx, badgeID, productID)
		} else {
			err = f.badges.RemoveProduct(txCtx, badgeID, productID)
		}
		if err != nil {
			return err
		}
		// The product is the resource here: an auditor asking what changed on a
		// product should find the assignment under it, not under the badge.
		return f.audit(txCtx, cmd, action, "product", productID, nil, payload)
	})
}

// ---------- reviews ----------

func (f *ContentAdminFacade) ModerateReview(ctx context.Context, cmd CatalogCommand, reviewID uuid.UUID, status reviewsDomain.Status) (*reviewsDomain.Review, error) {
	if err := f.authorize(ctx, cmd.ActorUserID, PermissionReviewsWrite, f.reviews == nil, "reviews"); err != nil {
		return nil, err
	}
	if reviewID == uuid.Nil {
		return nil, fmt.Errorf("review ID is required")
	}
	var moderated *reviewsDomain.Review
	err := f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		review, err := f.reviews.SetStatus(txCtx, reviewID, status)
		if err != nil {
			return err
		}
		moderated = review
		// Moderation decisions are the most contested admin action there is, so
		// the resulting status is recorded rather than only the intent.
		return f.audit(txCtx, cmd, "reviews.review.moderate", "review", reviewID, nil, map[string]any{"status": review.Status})
	})
	if err != nil {
		return nil, err
	}
	return moderated, nil
}

func (f *ContentAdminFacade) DeleteReview(ctx context.Context, cmd CatalogCommand, reviewID uuid.UUID) error {
	if err := f.authorize(ctx, cmd.ActorUserID, PermissionReviewsWrite, f.reviews == nil, "reviews"); err != nil {
		return err
	}
	if reviewID == uuid.Nil {
		return fmt.Errorf("review ID is required")
	}
	return f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := f.reviews.Delete(txCtx, reviewID); err != nil {
			return err
		}
		return f.audit(txCtx, cmd, "reviews.review.delete", "review", reviewID, nil, nil)
	})
}

// ---------- seo ----------

func (f *ContentAdminFacade) UpsertSEO(ctx context.Context, cmd CatalogCommand, command seoDomain.UpsertCommand) (*seoDomain.Metadata, error) {
	if err := f.authorize(ctx, cmd.ActorUserID, PermissionSEOWrite, f.seo == nil, "seo"); err != nil {
		return nil, err
	}
	var saved *seoDomain.Metadata
	err := f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		previous, _ := f.seo.Get(txCtx, command.ResourceType, command.ResourceID, command.Locale)
		metadata, err := f.seo.Upsert(txCtx, command)
		if err != nil {
			return err
		}
		saved = metadata
		return f.audit(txCtx, cmd, "seo.metadata.upsert", command.ResourceType, command.ResourceID, previous, metadata)
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

func (f *ContentAdminFacade) DeleteSEO(ctx context.Context, cmd CatalogCommand, resourceType string, resourceID uuid.UUID, locale string) error {
	if err := f.authorize(ctx, cmd.ActorUserID, PermissionSEOWrite, f.seo == nil, "seo"); err != nil {
		return err
	}
	if resourceID == uuid.Nil {
		return fmt.Errorf("SEO entity ID is required")
	}
	return f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		previous, _ := f.seo.Get(txCtx, resourceType, resourceID, locale)
		if err := f.seo.Delete(txCtx, resourceType, resourceID, locale); err != nil {
			return err
		}
		return f.audit(txCtx, cmd, "seo.metadata.delete", resourceType, resourceID, previous, map[string]any{"locale": locale})
	})
}

// ---------- shared ----------

func (f *ContentAdminFacade) authorize(ctx context.Context, actorID uuid.UUID, permission string, missing bool, module string) error {
	if f == nil || f.authorizer == nil {
		return fmt.Errorf("content admin facade is not configured")
	}
	if missing {
		return fmt.Errorf("%s module is not enabled", module)
	}
	if actorID == uuid.Nil {
		return adminDomain.ErrNotAdmin
	}
	return f.authorizer.Require(ctx, actorID, permission)
}

// audit records the change alongside it. Serializing the states here keeps
// every caller from repeating the marshal-and-sanitize dance.
func (f *ContentAdminFacade) audit(ctx context.Context, cmd CatalogCommand, action, resourceType string, resourceID uuid.UUID, before, after any) error {
	oldPayload, err := encodeAuditState(before)
	if err != nil {
		return fmt.Errorf("encode %s previous state: %w", action, err)
	}
	newPayload, err := encodeAuditState(after)
	if err != nil {
		return fmt.Errorf("encode %s new state: %w", action, err)
	}
	if cmd.EventKey == uuid.Nil {
		cmd.EventKey = uuid.New()
	}
	event, err := adminDomain.NewAdminActionEvent(
		cmd.EventKey, cmd.ActorUserID, action, resourceType, resourceID,
		oldPayload, newPayload, cmd.IPAddress, nil, nowUTC(),
	)
	if err != nil {
		return err
	}
	return f.publisher.Publish(ctx, event)
}

func encodeAuditState(state any) ([]byte, error) {
	if state == nil {
		return nil, nil
	}
	return json.Marshal(state)
}
