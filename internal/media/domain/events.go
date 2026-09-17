package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AssetUploadedEvent deliberately contains only the asset identity. Consumers
// read the owned snapshot, so delayed delivery cannot resurrect stale payloads.
type AssetUploadedEvent struct {
	AssetID  uuid.UUID
	UploadID uuid.UUID
	Occurred time.Time
}

func NewAssetUploadedEvent(uploadID, assetID uuid.UUID, occurredAt time.Time) (AssetUploadedEvent, error) {
	if uploadID == uuid.Nil || assetID == uuid.Nil {
		return AssetUploadedEvent{}, fmt.Errorf("media upload event requires upload and asset IDs")
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return AssetUploadedEvent{AssetID: assetID, UploadID: uploadID, Occurred: occurredAt.UTC()}, nil
}

func (AssetUploadedEvent) Topic() string               { return TopicAssetUploaded }
func (AssetUploadedEvent) AggregateType() string       { return "media_asset" }
func (e AssetUploadedEvent) AggregateID() uuid.UUID    { return e.AssetID }
func (e AssetUploadedEvent) IdempotencyKey() uuid.UUID { return e.UploadID }
func (e AssetUploadedEvent) OccurredAt() time.Time     { return e.Occurred }
func (e AssetUploadedEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(struct {
		Version int       `json:"version"`
		AssetID uuid.UUID `json:"asset_id"`
	}{Version: 1, AssetID: e.AssetID})
}
