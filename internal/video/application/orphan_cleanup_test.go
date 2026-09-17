package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

func TestOrphanCleanupDeletesProviderAssetOutsideDatabaseClaim(t *testing.T) {
	asset := video.Asset{ID: uuid.New(), Status: video.AssetUploading, ExternalID: "stream-stale"}
	repository := &cleanupRepository{assets: []video.Asset{asset}}
	provider := &cleanupProvider{}
	worker, err := NewOrphanCleanupWorker(repository, provider, discardCleanupLogger{})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if provider.deleted != "stream-stale" || repository.deleted != asset.ID {
		t.Fatalf("asset was not finalized after provider deletion: %+v %+v", provider, repository)
	}
}

func TestOrphanCleanupDeletesDraftWithoutCallingProvider(t *testing.T) {
	asset := video.Asset{ID: uuid.New(), Status: video.AssetDraft}
	repository := &cleanupRepository{assets: []video.Asset{asset}}
	provider := &cleanupProvider{}
	worker, _ := NewOrphanCleanupWorker(repository, provider, discardCleanupLogger{})
	if err := worker.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if provider.deleted != "" || repository.deleted != asset.ID {
		t.Fatalf("draft cleanup touched provider or did not finalize: %+v %+v", provider, repository)
	}
}

func TestOrphanCleanupRestoresLeaseWhenProviderDeleteFails(t *testing.T) {
	asset := video.Asset{ID: uuid.New(), Status: video.AssetProcessing, ExternalID: "stream-stale"}
	repository := &cleanupRepository{assets: []video.Asset{asset}}
	provider := &cleanupProvider{err: errors.New("provider unavailable")}
	worker, _ := NewOrphanCleanupWorker(repository, provider, discardCleanupLogger{})
	if err := worker.Cleanup(context.Background()); err == nil {
		t.Fatal("Cleanup() error = nil, want provider failure")
	}
	if repository.restored != asset.ID || repository.restoredStatus != video.AssetProcessing {
		t.Fatalf("cleanup lease was not restored: %+v", repository)
	}
}

type cleanupRepository struct {
	assets         []video.Asset
	deleted        uuid.UUID
	restored       uuid.UUID
	restoredStatus video.AssetStatus
}

func (*cleanupRepository) CreateDraft(context.Context, video.Asset) error         { return nil }
func (*cleanupRepository) MarkUploading(context.Context, uuid.UUID, string) error { return nil }
func (*cleanupRepository) MarkFailed(context.Context, uuid.UUID) error            { return nil }
func (*cleanupRepository) GetByExternalIDForUpdate(context.Context, string) (video.Asset, error) {
	return video.Asset{}, video.ErrAssetNotFound
}
func (*cleanupRepository) MarkReady(context.Context, uuid.UUID, int, string) error { return nil }
func (r *cleanupRepository) ClaimStaleForCleanup(context.Context, time.Time, time.Time, int) ([]video.Asset, error) {
	return r.assets, nil
}
func (r *cleanupRepository) MarkDeleted(_ context.Context, id uuid.UUID) error {
	r.deleted = id
	return nil
}
func (r *cleanupRepository) RestoreCleanup(_ context.Context, id uuid.UUID, status video.AssetStatus) error {
	r.restored, r.restoredStatus = id, status
	return nil
}

type cleanupProvider struct {
	deleted string
	err     error
}

func (*cleanupProvider) CreateDirectUpload(context.Context, video.DirectUploadMetadata) (video.UploadInstruction, error) {
	return video.UploadInstruction{}, errors.New("not used")
}
func (p *cleanupProvider) DeleteAsset(_ context.Context, externalID string) error {
	p.deleted = externalID
	return p.err
}
func (*cleanupProvider) VerifyWebhookSignature(context.Context, []byte, string) error { return nil }
func (*cleanupProvider) ParseWebhook(context.Context, []byte) (video.WebhookEvent, error) {
	return video.WebhookEvent{}, nil
}

type discardCleanupLogger struct{}

func (discardCleanupLogger) Errorw(string, ...any) {}
