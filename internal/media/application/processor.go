package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
)

// AssetUploadedHandler is intentionally a safe stub for 13.1. It owns the
// durable state transition; real image transforms are added behind ImageProcessor.
type AssetUploadedHandler struct {
	repository media.AssetRepository
	processor  media.ImageProcessor
}

func NewAssetUploadedHandler(repository media.AssetRepository, processors ...media.ImageProcessor) (*AssetUploadedHandler, error) {
	if repository == nil || len(processors) != 1 || processors[0] == nil {
		return nil, fmt.Errorf("media asset repository is required")
	}
	return &AssetUploadedHandler{repository: repository, processor: processors[0]}, nil
}
func (h *AssetUploadedHandler) Topic() string { return media.TopicAssetUploaded }
func (h *AssetUploadedHandler) Handle(ctx context.Context, delivery events.Delivery) error {
	var payload struct {
		Version int       `json:"version"`
		AssetID uuid.UUID `json:"asset_id"`
	}
	if delivery.EventID == uuid.Nil || delivery.AggregateID == uuid.Nil || delivery.Topic != media.TopicAssetUploaded {
		return fmt.Errorf("invalid media upload delivery")
	}
	if err := json.Unmarshal(delivery.Payload, &payload); err != nil || payload.Version != 1 || payload.AssetID == uuid.Nil || payload.AssetID != delivery.AggregateID {
		return fmt.Errorf("invalid media upload event payload")
	}
	asset, err := h.repository.Get(ctx, payload.AssetID)
	if err != nil {
		return err
	}
	objects, err := h.processor.Process(ctx, media.SourceObject{AssetID: asset.ID, Object: asset.Object, MIMEType: asset.MIMEType, SizeBytes: asset.SizeBytes, ChecksumSHA256: asset.ChecksumSHA256})
	if err != nil {
		return err
	}
	variants := make([]media.Variant, 0, len(objects))
	for _, object := range objects {
		key := "product"
		if strings.Contains(object.Key, "thumbnail.webp") {
			key = "thumbnail"
		}
		variants = append(variants, media.Variant{AssetID: asset.ID, Key: key, Object: object.ObjectRef, Width: object.Width, Height: object.Height, SizeBytes: object.SizeBytes, MIMEType: "image/webp"})
	}
	return h.repository.CompleteProcessing(ctx, payload.AssetID, delivery.EventID, variants)
}
