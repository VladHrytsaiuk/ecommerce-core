// Package domain defines provider-neutral video delivery contracts.
package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

var (
	ErrAssetNotFound           = errors.New("video asset not found")
	ErrInvalidWebhookSignature = errors.New("invalid video webhook signature")
	ErrInvalidWebhookPayload   = errors.New("invalid video webhook payload")
)

type AssetStatus string

const (
	AssetDraft      AssetStatus = "draft"
	AssetUploading  AssetStatus = "uploading"
	AssetProcessing AssetStatus = "processing"
	AssetReady      AssetStatus = "ready"
	AssetFailed     AssetStatus = "failed"
	// AssetDeleting is an internal reconciliation lease. It is never exposed
	// through the storefront and prevents two cleanup workers from deleting the
	// same provider asset concurrently.
	AssetDeleting AssetStatus = "deleting"
	AssetDeleted  AssetStatus = "deleted"
)

type Asset struct {
	ID              uuid.UUID
	Provider        string
	ExternalID      string
	Status          AssetStatus
	DurationSeconds int
	PosterURL       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type DirectUploadMetadata struct {
	AssetID            uuid.UUID
	MaxDurationSeconds int
}

type UploadInstruction struct {
	ExternalID string
	UploadURL  string
}

type WebhookOutcome string

const (
	WebhookEncodingSuccess WebhookOutcome = "success"
	WebhookEncodingError   WebhookOutcome = "error"
)

// WebhookEvent contains only the provider-normalized facts Video owns. It is
// intentionally not the Cloudflare JSON schema.
type WebhookEvent struct {
	ExternalID      string
	Outcome         WebhookOutcome
	DurationSeconds int
	PosterURL       string
	OccurredAt      time.Time
}

// VideoProvider is a narrow provider contract. No provider SDK leaks into
// application services or HTTP handlers.
type VideoProvider interface {
	CreateDirectUpload(context.Context, DirectUploadMetadata) (UploadInstruction, error)
	// DeleteAsset is used only by the orphan reconciler, after its database
	// lease has committed. Provider I/O must never happen inside a SQL
	// transaction.
	DeleteAsset(context.Context, string) error
	VerifyWebhookSignature(context.Context, []byte, string) error
	ParseWebhook(context.Context, []byte) (WebhookEvent, error)
}

type AssetRepository interface {
	CreateDraft(context.Context, Asset) error
	MarkUploading(context.Context, uuid.UUID, string) error
	MarkFailed(context.Context, uuid.UUID) error
	GetByExternalIDForUpdate(context.Context, string) (Asset, error)
	MarkReady(context.Context, uuid.UUID, int, string) error
	// ClaimStaleForCleanup atomically leases stale assets with SKIP LOCKED. The
	// returned assets are safe to delete from the external provider outside the
	// transaction that performed the claim.
	ClaimStaleForCleanup(context.Context, time.Time, time.Time, int) ([]Asset, error)
	MarkDeleted(context.Context, uuid.UUID) error
	RestoreCleanup(context.Context, uuid.UUID, AssetStatus) error
}

// Placement roles are a closed vocabulary mirrored by the product_videos CHECK
// constraint. Storefronts lay each role out differently, so an unknown value
// must be refused rather than rendered somewhere arbitrary.
const (
	RolePreview  = "preview"
	RoleHowToUse = "how_to_use"
)

var (
	ErrInvalidPlacement   = errors.New("invalid product video placement")
	ErrPlacementNotFound  = errors.New("product video placement not found")
	ErrAssetNotPlayable   = errors.New("video asset is not ready for placement")
	ErrPositionTaken      = errors.New("product video position is already occupied")
	ErrPlacementDuplicate = errors.New("video asset is already placed on this product")
)

func ValidRole(role string) bool {
	return role == RolePreview || role == RoleHowToUse
}

type ProductVideo struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	AssetID   uuid.UUID
	Role      string
	Position  int
	IsVisible bool
	Asset     Asset
}

// Placement is the administrator's intent for one product video slot.
type Placement struct {
	ProductID uuid.UUID
	AssetID   uuid.UUID
	Role      string
	Position  int
	IsVisible bool
}

func (p Placement) Validate() error {
	if p.ProductID == uuid.Nil || p.AssetID == uuid.Nil || !ValidRole(p.Role) || p.Position < 0 {
		return ErrInvalidPlacement
	}
	return nil
}

type StorefrontReader interface {
	ListReadyProductVideos(context.Context, uuid.UUID) ([]ProductVideo, error)
}

// Playback is a short-lived, provider-neutral grant to stream one asset. The
// URLs embed a token rather than the provider's asset identifier, so a link
// copied out of the page stops working instead of becoming a permanent public
// mirror of the video.
type Playback struct {
	HLSURL    string
	DASHURL   string
	ExpiresAt time.Time
}

// PlayableVideo is the storefront projection: a placement together with the
// grant that lets this viewer play it.
type PlayableVideo struct {
	ProductVideo
	Playback Playback
}

var ErrPlaybackUnavailable = errors.New("video playback could not be signed")

// PlaybackSigner mints those grants. It is separate from VideoProvider because
// signing needs a private key that upload and webhook verification do not.
type PlaybackSigner interface {
	SignPlayback(ctx context.Context, externalID string, expiresAt time.Time) (Playback, error)
}

// PlacementRepository owns the product_videos table. Attach refuses an asset
// that is not ready: a draft or failed encoding attached to a product would
// otherwise sit invisible until someone noticed the storefront gap.
type PlacementRepository interface {
	Attach(context.Context, Placement) (ProductVideo, error)
	Detach(ctx context.Context, productID, placementID uuid.UUID) error
	UpdatePlacement(ctx context.Context, productID, placementID uuid.UUID, position int, isVisible bool) (ProductVideo, error)
	// ListProductVideos returns every placement including hidden ones and
	// assets that are not ready, which is what an administrator needs to see.
	ListProductVideos(context.Context, uuid.UUID) ([]ProductVideo, error)
}

// TopicVideoReady records that Cloudflare finished encoding an asset, written
// in the same transaction as the status change it describes. It is an audit
// record of work done, not a work item: nothing consumes it and nothing is
// waiting to, because every read path already sees the asset as ready from that
// same transaction. Publishing it with no consumer is therefore the intended
// shape — the event is kept for the timeline, and no delivery row is created.
const TopicVideoReady = "media.video.ready.v1"

type VideoReadyEvent struct {
	VideoAssetID uuid.UUID
	ExternalID   string
	At           time.Time
}

func NewVideoReadyEvent(assetID uuid.UUID, externalID string, at time.Time) (VideoReadyEvent, error) {
	if assetID == uuid.Nil || externalID == "" {
		return VideoReadyEvent{}, fmt.Errorf("invalid video ready event")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return VideoReadyEvent{VideoAssetID: assetID, ExternalID: externalID, At: at.UTC()}, nil
}
func (VideoReadyEvent) Topic() string            { return TopicVideoReady }
func (VideoReadyEvent) AggregateType() string    { return "video_asset" }
func (e VideoReadyEvent) AggregateID() uuid.UUID { return e.VideoAssetID }
func (e VideoReadyEvent) IdempotencyKey() uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(e.VideoAssetID.String()+":ready"))
}
func (e VideoReadyEvent) OccurredAt() time.Time { return e.At }
func (e VideoReadyEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(struct {
		Version      int       `json:"version"`
		VideoAssetID uuid.UUID `json:"video_asset_id"`
		ExternalID   string    `json:"external_id"`
	}{Version: 1, VideoAssetID: e.VideoAssetID, ExternalID: e.ExternalID})
}

var _ events.DomainEvent = VideoReadyEvent{}
