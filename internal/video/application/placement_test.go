package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

func TestAttachPlacementAuditsInsideTheSameTransaction(t *testing.T) {
	repository := &placementRepositoryFake{}
	transaction := &observingTransaction{}
	// The publisher inspects the context it was given, so this assertion fails
	// if the audit event is ever published outside the mutation's transaction.
	publisher := &auditPublisher{}
	facade := newPlacementFacade(t, allowAll{}, repository, transaction, publisher)

	productID, assetID := uuid.New(), uuid.New()
	placed, err := facade.Attach(context.Background(), AttachPlacementCommand{
		ActorUserID: uuid.New(), ProductID: productID, AssetID: assetID,
		Role: video.RolePreview, Position: 0, IsVisible: true,
	})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	if placed.ProductID != productID || placed.AssetID != assetID {
		t.Fatalf("Attach() = %+v, want the requested placement", placed)
	}
	// A mutation recorded without its audit event would be a forensic bypass,
	// which is exactly why the router refuses to expose unaudited admin routes.
	if publisher.events != 1 || !publisher.insideTransaction {
		t.Fatalf("audit events = %d, inside transaction = %t; want 1 and true", publisher.events, publisher.insideTransaction)
	}
}

func TestAttachPlacementRollsBackWhenAuditFails(t *testing.T) {
	repository := &placementRepositoryFake{}
	publisher := &auditPublisher{err: errors.New("outbox unavailable")}
	transaction := &observingTransaction{}
	facade := newPlacementFacade(t, allowAll{}, repository, transaction, publisher)

	_, err := facade.Attach(context.Background(), AttachPlacementCommand{
		ActorUserID: uuid.New(), ProductID: uuid.New(), AssetID: uuid.New(),
		Role: video.RoleHowToUse, IsVisible: true,
	})
	if err == nil {
		t.Fatal("Attach() error = nil, want the audit failure to fail the mutation")
	}
	if !transaction.rolledBack {
		t.Fatal("transaction committed despite the audit event failing")
	}
}

func TestPlacementMutationsRequireVideoWritePermission(t *testing.T) {
	repository := &placementRepositoryFake{}
	facade := newPlacementFacade(t, denyAll{}, repository, &observingTransaction{}, &auditPublisher{})
	actorID, productID := uuid.New(), uuid.New()

	if _, err := facade.Attach(context.Background(), AttachPlacementCommand{ActorUserID: actorID, ProductID: productID, AssetID: uuid.New(), Role: video.RolePreview}); !errors.Is(err, adminDomain.ErrPermissionDenied) {
		t.Fatalf("Attach() error = %v, want permission denied", err)
	}
	if _, err := facade.Update(context.Background(), UpdatePlacementCommand{ActorUserID: actorID, ProductID: productID, PlacementID: uuid.New()}); !errors.Is(err, adminDomain.ErrPermissionDenied) {
		t.Fatalf("Update() error = %v, want permission denied", err)
	}
	if err := facade.Detach(context.Background(), DetachPlacementCommand{ActorUserID: actorID, ProductID: productID, PlacementID: uuid.New()}); !errors.Is(err, adminDomain.ErrPermissionDenied) {
		t.Fatalf("Detach() error = %v, want permission denied", err)
	}
	if _, err := facade.List(context.Background(), actorID, productID); !errors.Is(err, adminDomain.ErrPermissionDenied) {
		t.Fatalf("List() error = %v, want permission denied", err)
	}
	if repository.attached+repository.detached+repository.updated != 0 {
		t.Fatal("a denied request still reached the repository")
	}
}

func TestPlacementRejectsAnonymousActor(t *testing.T) {
	facade := newPlacementFacade(t, allowAll{}, &placementRepositoryFake{}, &observingTransaction{}, &auditPublisher{})

	// uuid.Nil would otherwise be audited as a real administrator's action.
	if _, err := facade.Attach(context.Background(), AttachPlacementCommand{ProductID: uuid.New(), AssetID: uuid.New(), Role: video.RolePreview}); !errors.Is(err, adminDomain.ErrNotAdmin) {
		t.Fatalf("Attach() error = %v, want ErrNotAdmin", err)
	}
}

func TestPlacementRejectsUnknownRoleBeforeTouchingTheDatabase(t *testing.T) {
	repository := &placementRepositoryFake{}
	facade := newPlacementFacade(t, allowAll{}, repository, &observingTransaction{}, &auditPublisher{})

	// product_videos constrains role, so an unknown value would fail deep in
	// the driver instead of as a clear validation error.
	if _, err := facade.Attach(context.Background(), AttachPlacementCommand{
		ActorUserID: uuid.New(), ProductID: uuid.New(), AssetID: uuid.New(), Role: "hero_banner",
	}); !errors.Is(err, video.ErrInvalidPlacement) {
		t.Fatalf("Attach() error = %v, want ErrInvalidPlacement", err)
	}
	if repository.attached != 0 {
		t.Fatal("an invalid role reached the repository")
	}
}

func TestPlacementReusesSuppliedEventKeyForAudit(t *testing.T) {
	publisher := &auditPublisher{}
	facade := newPlacementFacade(t, allowAll{}, &placementRepositoryFake{}, &observingTransaction{}, publisher)
	key := uuid.New()

	// The Outbox deduplicates on the event key, so a retried request must not
	// produce a second audit entry for the same change.
	for range 2 {
		if _, err := facade.Attach(context.Background(), AttachPlacementCommand{
			ActorUserID: uuid.New(), EventKey: key, ProductID: uuid.New(),
			AssetID: uuid.New(), Role: video.RolePreview,
		}); err != nil {
			t.Fatalf("Attach() error = %v", err)
		}
	}
	if len(publisher.keys) != 2 || publisher.keys[0] != key || publisher.keys[1] != key {
		t.Fatalf("audit event keys = %v, want both to be the supplied key", publisher.keys)
	}
}

func newPlacementFacade(t *testing.T, authorizer adminDomain.Authorizer, repository video.PlacementRepository, tx TransactionManager, publisher events.TransactionalEventPublisher) *PlacementAdminFacade {
	t.Helper()
	facade, err := NewPlacementAdminFacade(authorizer, repository, tx, publisher)
	if err != nil {
		t.Fatalf("NewPlacementAdminFacade() error = %v", err)
	}
	return facade
}

type allowAll struct{}

func (allowAll) Require(context.Context, uuid.UUID, string) error { return nil }

type denyAll struct{}

func (denyAll) Require(context.Context, uuid.UUID, string) error {
	return adminDomain.ErrPermissionDenied
}

type placementRepositoryFake struct {
	attached, detached, updated int
}

func (r *placementRepositoryFake) Attach(_ context.Context, placement video.Placement) (video.ProductVideo, error) {
	r.attached++
	return video.ProductVideo{ID: uuid.New(), ProductID: placement.ProductID, AssetID: placement.AssetID, Role: placement.Role, Position: placement.Position, IsVisible: placement.IsVisible}, nil
}

func (r *placementRepositoryFake) Detach(context.Context, uuid.UUID, uuid.UUID) error {
	r.detached++
	return nil
}

func (r *placementRepositoryFake) UpdatePlacement(_ context.Context, productID, placementID uuid.UUID, position int, isVisible bool) (video.ProductVideo, error) {
	r.updated++
	return video.ProductVideo{ID: placementID, ProductID: productID, Position: position, IsVisible: isVisible}, nil
}

func (r *placementRepositoryFake) ListProductVideos(context.Context, uuid.UUID) ([]video.ProductVideo, error) {
	return nil, nil
}

type transactionMarker struct{}

// observingTransaction stamps the context it hands to the unit of work. The
// production publisher resolves its transaction from the context, so checking
// for that stamp is what proves the audit event is written through the same
// transaction rather than merely during it.
type observingTransaction struct {
	rolledBack bool
}

func (t *observingTransaction) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	err := fn(context.WithValue(ctx, transactionMarker{}, true))
	if err != nil {
		t.rolledBack = true
	}
	return err
}

type auditPublisher struct {
	events            int
	keys              []uuid.UUID
	err               error
	insideTransaction bool
}

func (p *auditPublisher) Publish(ctx context.Context, event events.DomainEvent) error {
	if p.err != nil {
		return p.err
	}
	p.events++
	p.keys = append(p.keys, event.IdempotencyKey())
	p.insideTransaction, _ = ctx.Value(transactionMarker{}).(bool)
	return nil
}

func TestStorefrontSkipsAVideoItCannotSign(t *testing.T) {
	readable := video.ProductVideo{ID: uuid.New(), Asset: video.Asset{ExternalID: "ok", Status: video.AssetReady}}
	unsignable := video.ProductVideo{ID: uuid.New(), Asset: video.Asset{ExternalID: "broken", Status: video.AssetReady}}
	service, err := NewStorefrontService(
		staticStorefront{videos: []video.ProductVideo{readable, unsignable}},
		selectiveSigner{failFor: "broken"},
		time.Hour,
	)
	if err != nil {
		t.Fatalf("NewStorefrontService() error = %v", err)
	}

	playable, err := service.ListPlayableProductVideos(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("ListPlayableProductVideos() error = %v", err)
	}
	// One unplayable video must degrade that video, not hide the product's
	// whole media section behind an error.
	if len(playable) != 1 || playable[0].ID != readable.ID {
		t.Fatalf("playable = %+v, want only the signable video", playable)
	}
}

func TestStorefrontGrantsExpireAfterTheConfiguredTTL(t *testing.T) {
	service, err := NewStorefrontService(
		staticStorefront{videos: []video.ProductVideo{{ID: uuid.New(), Asset: video.Asset{ExternalID: "ok", Status: video.AssetReady}}}},
		selectiveSigner{},
		15*time.Minute,
	)
	if err != nil {
		t.Fatalf("NewStorefrontService() error = %v", err)
	}
	issuedAt := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return issuedAt }

	playable, err := service.ListPlayableProductVideos(context.Background(), uuid.New())
	if err != nil || len(playable) != 1 {
		t.Fatalf("ListPlayableProductVideos() = (%d videos, %v)", len(playable), err)
	}
	if !playable[0].Playback.ExpiresAt.Equal(issuedAt.Add(15 * time.Minute)) {
		t.Fatalf("grant expiry = %s, want the configured TTL from issue time", playable[0].Playback.ExpiresAt)
	}
}

func TestNewStorefrontServiceRejectsAnUnboundedTTL(t *testing.T) {
	// A non-positive TTL would mint grants that never usefully expire.
	for _, ttl := range []time.Duration{0, -time.Minute} {
		if _, err := NewStorefrontService(staticStorefront{}, selectiveSigner{}, ttl); err == nil {
			t.Fatalf("NewStorefrontService(ttl=%s) error = nil, want refusal", ttl)
		}
	}
}

type staticStorefront struct{ videos []video.ProductVideo }

func (s staticStorefront) ListReadyProductVideos(context.Context, uuid.UUID) ([]video.ProductVideo, error) {
	return s.videos, nil
}

type selectiveSigner struct{ failFor string }

func (s selectiveSigner) SignPlayback(_ context.Context, externalID string, expiresAt time.Time) (video.Playback, error) {
	if externalID == s.failFor {
		return video.Playback{}, video.ErrPlaybackUnavailable
	}
	return video.Playback{HLSURL: "https://stream.example.test/token/manifest/video.m3u8", ExpiresAt: expiresAt}, nil
}
