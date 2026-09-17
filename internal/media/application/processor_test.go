package application

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
)

func TestAssetUploadedHandlerRejectsInvalidEventContractBeforeRepositoryAccess(t *testing.T) {
	assetID := uuid.New()
	validPayload, err := json.Marshal(map[string]any{"version": 1, "asset_id": assetID})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		delivery events.Delivery
	}{
		{name: "wrong topic", delivery: events.Delivery{EventID: uuid.New(), Topic: "media.asset.uploaded.v2", AggregateID: assetID, Payload: validPayload}},
		{name: "unsupported version", delivery: events.Delivery{EventID: uuid.New(), Topic: media.TopicAssetUploaded, AggregateID: assetID, Payload: []byte(`{"version":2,"asset_id":"` + assetID.String() + `"}`)}},
		{name: "aggregate mismatch", delivery: events.Delivery{EventID: uuid.New(), Topic: media.TopicAssetUploaded, AggregateID: uuid.New(), Payload: validPayload}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &trackingAssetRepository{}
			handler, err := NewAssetUploadedHandler(repository, fakeImageProcessor{})
			if err != nil {
				t.Fatal(err)
			}
			if err := handler.Handle(context.Background(), tt.delivery); err == nil {
				t.Fatal("Handle() error = nil, want contract validation error")
			}
			if repository.getCalls != 0 {
				t.Fatalf("repository.Get calls = %d, want 0", repository.getCalls)
			}
		})
	}
}

type trackingAssetRepository struct {
	fakeRepository
	getCalls int
}

func (r *trackingAssetRepository) Get(ctx context.Context, id uuid.UUID) (media.Asset, error) {
	r.getCalls++
	return r.fakeRepository.Get(ctx, id)
}

type fakeImageProcessor struct{}

func (fakeImageProcessor) Process(context.Context, media.SourceObject) ([]media.StoredObject, error) {
	return nil, nil
}
