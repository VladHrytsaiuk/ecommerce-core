package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

// PlacementAdminFacade is the sole Admin-to-Video mutation boundary for
// product video placements. Authorization, the mutation and the audit event
// share one database transaction, so a change can never be applied without its
// forensic record — the reason the router refuses to expose unaudited
// permission-protected routes at all.
type PlacementAdminFacade struct {
	authorizer    adminDomain.Authorizer
	placements    video.PlacementRepository
	tx            TransactionManager
	publisher     events.TransactionalEventPublisher
	now           func() time.Time
	newIdentifier func() uuid.UUID
}

func NewPlacementAdminFacade(authorizer adminDomain.Authorizer, placements video.PlacementRepository, tx TransactionManager, publisher events.TransactionalEventPublisher) (*PlacementAdminFacade, error) {
	if authorizer == nil || placements == nil || tx == nil || publisher == nil {
		return nil, fmt.Errorf("video placement admin facade is not configured")
	}
	return &PlacementAdminFacade{
		authorizer: authorizer, placements: placements, tx: tx, publisher: publisher,
		now:           func() time.Time { return time.Now().UTC() },
		newIdentifier: uuid.New,
	}, nil
}

type AttachPlacementCommand struct {
	ActorUserID uuid.UUID
	EventKey    uuid.UUID
	ProductID   uuid.UUID
	AssetID     uuid.UUID
	Role        string
	Position    int
	IsVisible   bool
}

type UpdatePlacementCommand struct {
	ActorUserID uuid.UUID
	EventKey    uuid.UUID
	ProductID   uuid.UUID
	PlacementID uuid.UUID
	Position    int
	IsVisible   bool
}

type DetachPlacementCommand struct {
	ActorUserID uuid.UUID
	EventKey    uuid.UUID
	ProductID   uuid.UUID
	PlacementID uuid.UUID
}

func (f *PlacementAdminFacade) Attach(ctx context.Context, command AttachPlacementCommand) (video.ProductVideo, error) {
	if err := f.authorize(ctx, command.ActorUserID); err != nil {
		return video.ProductVideo{}, err
	}
	placement := video.Placement{ProductID: command.ProductID, AssetID: command.AssetID, Role: command.Role, Position: command.Position, IsVisible: command.IsVisible}
	if err := placement.Validate(); err != nil {
		return video.ProductVideo{}, err
	}
	var attached video.ProductVideo
	err := f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		created, err := f.placements.Attach(txCtx, placement)
		if err != nil {
			return err
		}
		attached = created
		return f.audit(txCtx, command.ActorUserID, f.eventKey(command.EventKey), "video.placement.attached", created.ID, map[string]any{
			"product_id": created.ProductID, "video_asset_id": created.AssetID,
			"role": created.Role, "position": created.Position, "is_visible": created.IsVisible,
		})
	})
	if err != nil {
		return video.ProductVideo{}, err
	}
	return attached, nil
}

func (f *PlacementAdminFacade) Update(ctx context.Context, command UpdatePlacementCommand) (video.ProductVideo, error) {
	if err := f.authorize(ctx, command.ActorUserID); err != nil {
		return video.ProductVideo{}, err
	}
	if command.ProductID == uuid.Nil || command.PlacementID == uuid.Nil || command.Position < 0 {
		return video.ProductVideo{}, video.ErrInvalidPlacement
	}
	var updated video.ProductVideo
	err := f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		result, err := f.placements.UpdatePlacement(txCtx, command.ProductID, command.PlacementID, command.Position, command.IsVisible)
		if err != nil {
			return err
		}
		updated = result
		return f.audit(txCtx, command.ActorUserID, f.eventKey(command.EventKey), "video.placement.updated", result.ID, map[string]any{
			"product_id": result.ProductID, "position": result.Position, "is_visible": result.IsVisible,
		})
	})
	if err != nil {
		return video.ProductVideo{}, err
	}
	return updated, nil
}

func (f *PlacementAdminFacade) Detach(ctx context.Context, command DetachPlacementCommand) error {
	if err := f.authorize(ctx, command.ActorUserID); err != nil {
		return err
	}
	if command.ProductID == uuid.Nil || command.PlacementID == uuid.Nil {
		return video.ErrInvalidPlacement
	}
	return f.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := f.placements.Detach(txCtx, command.ProductID, command.PlacementID); err != nil {
			return err
		}
		return f.audit(txCtx, command.ActorUserID, f.eventKey(command.EventKey), "video.placement.detached", command.PlacementID, map[string]any{
			"product_id": command.ProductID,
		})
	})
}

// List is a read, so it needs authorization but no transaction or audit entry.
func (f *PlacementAdminFacade) List(ctx context.Context, actorUserID, productID uuid.UUID) ([]video.ProductVideo, error) {
	if err := f.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	if productID == uuid.Nil {
		return nil, video.ErrInvalidPlacement
	}
	return f.placements.ListProductVideos(ctx, productID)
}

func (f *PlacementAdminFacade) authorize(ctx context.Context, actorUserID uuid.UUID) error {
	if actorUserID == uuid.Nil {
		return adminDomain.ErrNotAdmin
	}
	return f.authorizer.Require(ctx, actorUserID, PermissionVideoWrite)
}

// eventKey lets a retried request reuse its audit identity, so the Outbox
// deduplicates a replayed mutation instead of recording it twice.
func (f *PlacementAdminFacade) eventKey(provided uuid.UUID) uuid.UUID {
	if provided != uuid.Nil {
		return provided
	}
	return f.newIdentifier()
}

func (f *PlacementAdminFacade) audit(ctx context.Context, actorUserID, eventKey uuid.UUID, action string, resourceID uuid.UUID, payload map[string]any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode %s audit payload: %w", action, err)
	}
	// The placement mutation carries no prior state worth diffing and no
	// client IP, so those arguments stay empty rather than being invented.
	event, err := adminDomain.NewAdminActionEvent(
		eventKey, actorUserID, action, "product_video", resourceID,
		nil, encoded, "", nil, f.now(),
	)
	if err != nil {
		return err
	}
	return f.publisher.Publish(ctx, event)
}
