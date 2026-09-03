package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

func TestWebhookSuccessUpdatesAssetAndPublishesInOneTransaction(t *testing.T) {
	assetID := uuid.New()
	repository := &webhookRepository{asset: video.Asset{ID: assetID, ExternalID: "stream-id", Status: video.AssetUploading}}
	provider := webhookProvider{event: video.WebhookEvent{ExternalID: "stream-id", Outcome: video.WebhookEncodingSuccess, DurationSeconds: 42, PosterURL: "https://video.example.test/poster.jpg", OccurredAt: time.Now()}}
	publisher := &recordingPublisher{}
	service, err := NewWebhookService(repository, provider, immediateTransaction{}, publisher)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Handle(context.Background(), []byte(`{"uid":"stream-id"}`), "valid"); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if repository.readyID != assetID || repository.duration != 42 || publisher.events != 1 {
		t.Fatalf("unexpected result: %+v events=%d", repository, publisher.events)
	}
}

func TestWebhookTerminalAssetIsIdempotent(t *testing.T) {
	repository := &webhookRepository{asset: video.Asset{ID: uuid.New(), ExternalID: "stream-id", Status: video.AssetReady}}
	publisher := &recordingPublisher{}
	service, err := NewWebhookService(repository, webhookProvider{event: video.WebhookEvent{ExternalID: "stream-id", Outcome: video.WebhookEncodingSuccess, PosterURL: "https://video.example.test/poster.jpg"}}, immediateTransaction{}, publisher)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Handle(context.Background(), []byte(`{"uid":"stream-id"}`), "valid"); err != nil {
		t.Fatal(err)
	}
	if repository.readyID != uuid.Nil || publisher.events != 0 {
		t.Fatalf("terminal asset was processed again")
	}
}

func TestWebhookRepeatedSuccessPublishesReadyEventOnce(t *testing.T) {
	assetID := uuid.New()
	repository := &webhookRepository{asset: video.Asset{ID: assetID, ExternalID: "stream-id", Status: video.AssetUploading}}
	publisher := &recordingPublisher{}
	service, err := NewWebhookService(repository, webhookProvider{event: video.WebhookEvent{ExternalID: "stream-id", Outcome: video.WebhookEncodingSuccess, DurationSeconds: 2, PosterURL: "https://video.example.test/poster.jpg"}}, immediateTransaction{}, publisher)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Handle(context.Background(), []byte(`{"uid":"stream-id"}`), "valid"); err != nil {
		t.Fatal(err)
	}
	if err := service.Handle(context.Background(), []byte(`{"uid":"stream-id"}`), "valid"); err != nil {
		t.Fatal(err)
	}
	if publisher.events != 1 || repository.asset.Status != video.AssetReady {
		t.Fatalf("replayed webhook was not idempotent: events=%d asset=%+v", publisher.events, repository.asset)
	}
}

func TestWebhookErrorMarksUploadingAssetFailedWithoutReadyEvent(t *testing.T) {
	assetID := uuid.New()
	repository := &webhookRepository{asset: video.Asset{ID: assetID, ExternalID: "stream-id", Status: video.AssetUploading}}
	publisher := &recordingPublisher{}
	service, err := NewWebhookService(repository, webhookProvider{event: video.WebhookEvent{ExternalID: "stream-id", Outcome: video.WebhookEncodingError}}, immediateTransaction{}, publisher)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Handle(context.Background(), []byte(`{"uid":"stream-id"}`), "valid"); err != nil {
		t.Fatal(err)
	}
	if repository.failedID != assetID || publisher.events != 0 {
		t.Fatalf("error event had unexpected effects")
	}
}

type immediateTransaction struct{}

func (immediateTransaction) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type webhookRepository struct {
	asset    video.Asset
	readyID  uuid.UUID
	duration int
	failedID uuid.UUID
}

func (*webhookRepository) CreateDraft(context.Context, video.Asset) error         { return nil }
func (*webhookRepository) MarkUploading(context.Context, uuid.UUID, string) error { return nil }
func (r *webhookRepository) MarkFailed(_ context.Context, id uuid.UUID) error {
	r.failedID = id
	return nil
}
func (r *webhookRepository) GetByExternalIDForUpdate(_ context.Context, externalID string) (video.Asset, error) {
	if externalID != r.asset.ExternalID {
		return video.Asset{}, video.ErrAssetNotFound
	}
	return r.asset, nil
}
func (r *webhookRepository) MarkReady(_ context.Context, id uuid.UUID, duration int, _ string) error {
	r.readyID, r.duration = id, duration
	r.asset.Status = video.AssetReady
	return nil
}
func (*webhookRepository) ClaimStaleForCleanup(context.Context, time.Time, time.Time, int) ([]video.Asset, error) {
	return nil, nil
}
func (*webhookRepository) MarkDeleted(context.Context, uuid.UUID) error { return nil }
func (*webhookRepository) RestoreCleanup(context.Context, uuid.UUID, video.AssetStatus) error {
	return nil
}

type webhookProvider struct {
	event     video.WebhookEvent
	verifyErr error
}

func (webhookProvider) CreateDirectUpload(context.Context, video.DirectUploadMetadata) (video.UploadInstruction, error) {
	return video.UploadInstruction{}, errors.New("not used")
}
func (p webhookProvider) VerifyWebhookSignature(context.Context, []byte, string) error {
	return p.verifyErr
}
func (webhookProvider) DeleteAsset(context.Context, string) error { return nil }
func (p webhookProvider) ParseWebhook(context.Context, []byte) (video.WebhookEvent, error) {
	return p.event, nil
}

type recordingPublisher struct{ events int }

func (p *recordingPublisher) Publish(_ context.Context, event events.DomainEvent) error {
	if event.Topic() != video.TopicVideoReady {
		return errors.New("unexpected topic")
	}
	p.events++
	return nil
}
