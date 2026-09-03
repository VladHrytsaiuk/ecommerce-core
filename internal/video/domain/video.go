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

type ProductVideo struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	AssetID   uuid.UUID
	Role      string
	Position  int
	Asset     Asset
}

type StorefrontReader interface {
	ListReadyProductVideos(context.Context, uuid.UUID) ([]ProductVideo, error)
}

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
