package application

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
)

func TestOrphanCleanupDeletesOnlyOldUnreferencedOrFailedQuarantineObjects(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	store := &cleanupStore{objects: []media.ListedObject{
		{ObjectRef: media.ObjectRef{Key: "quarantine/referenced"}, LastModified: now.Add(-25 * time.Hour)},
		{ObjectRef: media.ObjectRef{Key: "quarantine/orphan"}, LastModified: now.Add(-25 * time.Hour)},
		{ObjectRef: media.ObjectRef{Key: "quarantine/recent"}, LastModified: now.Add(-time.Hour)},
	}}
	repository := &cleanupRepository{referenced: map[string]bool{"quarantine/referenced": true}}
	worker, err := NewOrphanCleanupWorker(repository, store, cleanupLogger{})
	if err != nil {
		t.Fatal(err)
	}
	worker.now = func() time.Time { return now }
	if err := worker.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0].Key != "quarantine/orphan" {
		t.Fatalf("deleted = %+v; want only old orphan", store.deleted)
	}
}

type cleanupLogger struct{}

func (cleanupLogger) Errorw(string, ...any) {}

type cleanupStore struct {
	objects []media.ListedObject
	deleted []media.ObjectRef
}

func (s *cleanupStore) Put(context.Context, media.PutRequest) (media.StoredObject, error) {
	return media.StoredObject{}, nil
}
func (*cleanupStore) Get(context.Context, media.ObjectRef) (io.ReadCloser, error) { return nil, nil }
func (s *cleanupStore) List(context.Context, string) ([]media.ListedObject, error) {
	return s.objects, nil
}
func (s *cleanupStore) Delete(_ context.Context, object media.ObjectRef) error {
	s.deleted = append(s.deleted, object)
	return nil
}
func (*cleanupStore) PublicURL(context.Context, media.ObjectRef) (string, error) { return "", nil }

type cleanupRepository struct{ referenced map[string]bool }

func (*cleanupRepository) Create(context.Context, media.Asset) error { return nil }
func (*cleanupRepository) Get(context.Context, uuid.UUID) (media.Asset, error) {
	return media.Asset{}, nil
}
func (*cleanupRepository) GetByUploadID(context.Context, uuid.UUID) (media.Asset, error) {
	return media.Asset{}, nil
}
func (*cleanupRepository) CompleteProcessing(context.Context, uuid.UUID, uuid.UUID, []media.Variant) error {
	return nil
}
func (*cleanupRepository) CheckAssetsReady(context.Context, []uuid.UUID) error { return nil }
func (*cleanupRepository) VariantsForAssets(context.Context, []uuid.UUID) (map[uuid.UUID][]media.Variant, error) {
	return nil, nil
}
func (r *cleanupRepository) ReferencedObjectKeys(_ context.Context, keys []string) (map[string]bool, error) {
	result := make(map[string]bool, len(keys))
	for _, key := range keys {
		result[key] = r.referenced[key]
	}
	return result, nil
}
