package application

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"

	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
)

func TestUploadWritesQuarantineAndEventInsideTransaction(t *testing.T) {
	store := &fakeStore{}
	repository := &fakeRepository{}
	publisher := &fakePublisher{}
	service, err := NewUploadService(repository, store, fakeTx{}, publisher, "minio", "media")
	if err != nil {
		t.Fatal(err)
	}
	asset, err := service.Upload(context.Background(), UploadCommand{UploadID: uuid.New(), Body: bytes.NewReader([]byte("image")), SizeBytes: 5, MIMEType: "image/png", ChecksumSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if asset.Status != media.AssetQuarantine || repository.created.ID != asset.ID || publisher.event == nil {
		t.Fatalf("asset/event = %+v / %+v", repository.created, publisher.event)
	}
	if store.deleted {
		t.Fatal("stored object was unexpectedly deleted")
	}
}

func TestUploadDeletesObjectWhenTransactionalOutboxFails(t *testing.T) {
	store := &fakeStore{}
	service, err := NewUploadService(&fakeRepository{}, store, fakeTx{}, &fakePublisher{err: errors.New("outbox unavailable")}, "s3", "media")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Upload(context.Background(), UploadCommand{UploadID: uuid.New(), Body: bytes.NewReader([]byte("image")), SizeBytes: 5, MIMEType: "image/png", ChecksumSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err == nil || !store.deleted {
		t.Fatalf("Upload() = %v, deleted=%t; want rollback cleanup", err, store.deleted)
	}
}

func TestUploadReusesExistingAssetForSameIdempotencyKey(t *testing.T) {
	asset := media.Asset{ID: uuid.New(), UploadID: uuid.New(), Status: media.AssetReady}
	repository := &fakeRepository{existing: asset}
	store := &fakeStore{}
	service, err := NewUploadService(repository, store, fakeTx{}, &fakePublisher{}, "s3", "media")
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.Upload(context.Background(), UploadCommand{UploadID: asset.UploadID, Body: bytes.NewReader([]byte("image")), SizeBytes: 5, MIMEType: "image/png", ChecksumSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil || got.ID != asset.ID || store.puts != 0 {
		t.Fatalf("Upload() = (%+v, %v), puts=%d; want existing asset without store write", got, err, store.puts)
	}
}

type fakeTx struct{}

func (fakeTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeStore struct {
	deleted bool
	puts    int
}

func (s *fakeStore) Put(_ context.Context, req media.PutRequest) (media.StoredObject, error) {
	s.puts++
	_, _ = io.ReadAll(req.Body)
	return media.StoredObject{ObjectRef: req.ObjectRef, SizeBytes: req.ContentLength}, nil
}
func (*fakeStore) Get(context.Context, media.ObjectRef) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (*fakeStore) List(context.Context, string) ([]media.ListedObject, error)   { return nil, nil }
func (s *fakeStore) Delete(context.Context, media.ObjectRef) error              { s.deleted = true; return nil }
func (s *fakeStore) PublicURL(context.Context, media.ObjectRef) (string, error) { return "", nil }

type fakeRepository struct{ created, existing media.Asset }

func (r *fakeRepository) Create(_ context.Context, asset media.Asset) error {
	r.created = asset
	return nil
}
func (*fakeRepository) Get(context.Context, uuid.UUID) (media.Asset, error) {
	return media.Asset{}, nil
}
func (r *fakeRepository) GetByUploadID(_ context.Context, id uuid.UUID) (media.Asset, error) {
	if r.existing.UploadID == id {
		return r.existing, nil
	}
	return media.Asset{}, media.ErrAssetNotFound
}
func (*fakeRepository) CompleteProcessing(context.Context, uuid.UUID, uuid.UUID, []media.Variant) error {
	return nil
}
func (*fakeRepository) CheckAssetsReady(context.Context, []uuid.UUID) error { return nil }
func (*fakeRepository) VariantsForAssets(context.Context, []uuid.UUID) (map[uuid.UUID][]media.Variant, error) {
	return nil, nil
}
func (*fakeRepository) ReferencedObjectKeys(context.Context, []string) (map[string]bool, error) {
	return nil, nil
}

type fakePublisher struct {
	event events.DomainEvent
	err   error
}

func (p *fakePublisher) Publish(_ context.Context, event events.DomainEvent) error {
	p.event = event
	return p.err
}
