package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

func TestCreateDirectUploadPersistsDraftBeforeProviderAndNeverPassesTransaction(t *testing.T) {
	repository := &recordingRepository{}
	provider := &recordingProvider{repository: repository}
	service, err := NewDirectUploadService(repository, provider, "cloudflare")
	if err != nil {
		t.Fatal(err)
	}

	asset, instruction, err := service.CreateDirectUpload(context.Background(), CreateDirectUploadCommand{MaxDurationSeconds: 120})
	if err != nil {
		t.Fatalf("CreateDirectUpload() error = %v", err)
	}
	if !provider.sawDraft || asset.Status != video.AssetUploading || instruction.ExternalID == "" || repository.uploadingID != asset.ID {
		t.Fatalf("unexpected direct upload state: %+v %+v", asset, instruction)
	}
}

func TestCreateDirectUploadMarksDraftFailedWhenProviderFails(t *testing.T) {
	repository := &recordingRepository{}
	service, err := NewDirectUploadService(repository, failingProvider{}, "cloudflare")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.CreateDirectUpload(context.Background(), CreateDirectUploadCommand{}); err == nil {
		t.Fatal("CreateDirectUpload() error = nil, want provider failure")
	}
	if repository.failedID == uuid.Nil {
		t.Fatal("provider failure did not mark draft failed")
	}
}

type recordingRepository struct {
	draft                 video.Asset
	uploadingID, failedID uuid.UUID
}

func (r *recordingRepository) CreateDraft(_ context.Context, asset video.Asset) error {
	r.draft = asset
	return nil
}
func (r *recordingRepository) MarkUploading(_ context.Context, id uuid.UUID, _ string) error {
	r.uploadingID = id
	return nil
}
func (r *recordingRepository) MarkFailed(_ context.Context, id uuid.UUID) error {
	r.failedID = id
	return nil
}
func (*recordingRepository) GetByExternalIDForUpdate(context.Context, string) (video.Asset, error) {
	return video.Asset{}, video.ErrAssetNotFound
}
func (*recordingRepository) MarkReady(context.Context, uuid.UUID, int, string) error { return nil }
func (*recordingRepository) ClaimStaleForCleanup(context.Context, time.Time, time.Time, int) ([]video.Asset, error) {
	return nil, nil
}
func (*recordingRepository) MarkDeleted(context.Context, uuid.UUID) error { return nil }
func (*recordingRepository) RestoreCleanup(context.Context, uuid.UUID, video.AssetStatus) error {
	return nil
}

type recordingProvider struct {
	repository *recordingRepository
	sawDraft   bool
}

func (p *recordingProvider) CreateDirectUpload(_ context.Context, metadata video.DirectUploadMetadata) (video.UploadInstruction, error) {
	p.sawDraft = p.repository.draft.ID == metadata.AssetID && p.repository.draft.Status == video.AssetDraft
	return video.UploadInstruction{ExternalID: "stream-id", UploadURL: "https://upload.example.test/tus"}, nil
}
func (*recordingProvider) VerifyWebhookSignature(context.Context, []byte, string) error { return nil }
func (*recordingProvider) DeleteAsset(context.Context, string) error                    { return nil }
func (*recordingProvider) ParseWebhook(context.Context, []byte) (video.WebhookEvent, error) {
	return video.WebhookEvent{}, nil
}

type failingProvider struct{}

func (failingProvider) CreateDirectUpload(context.Context, video.DirectUploadMetadata) (video.UploadInstruction, error) {
	return video.UploadInstruction{}, errors.New("provider unavailable")
}
func (failingProvider) VerifyWebhookSignature(context.Context, []byte, string) error { return nil }
func (failingProvider) DeleteAsset(context.Context, string) error                    { return nil }
func (failingProvider) ParseWebhook(context.Context, []byte) (video.WebhookEvent, error) {
	return video.WebhookEvent{}, nil
}
