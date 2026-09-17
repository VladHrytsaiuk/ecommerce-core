package application

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"

	"github.com/google/uuid"

	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
)

func TestValidateImageConfigRejectsDecompressionBombBeforePixelDecode(t *testing.T) {
	err := validateImageConfig(image.Config{Width: 25_000, Height: 25_000})
	if err == nil {
		t.Fatal("validateImageConfig() error = nil, want dimension/pixel limit rejection")
	}
}

func TestPureGoProcessorReopensObjectOnlyAfterConfigValidation(t *testing.T) {
	var source bytes.Buffer
	imageData := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	imageData.Set(0, 0, color.White)
	if err := png.Encode(&source, imageData); err != nil {
		t.Fatal(err)
	}
	store := &processorStore{source: source.Bytes()}
	processor := NewPureGoProcessor(store)
	objects, err := processor.Process(context.Background(), media.SourceObject{AssetID: uuid.New(), Object: media.ObjectRef{Provider: "minio", Bucket: "media", Key: "quarantine/a"}})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if store.gets != 2 || len(objects) != 2 || store.puts != 2 {
		t.Fatalf("S3 operations gets=%d puts=%d objects=%d; want config read + decode read and two variants", store.gets, store.puts, len(objects))
	}
	for _, object := range objects {
		if object.SizeBytes <= 0 || object.Width <= 0 || object.Height <= 0 {
			t.Fatalf("variant metadata = %+v; want non-zero dimensions and size", object)
		}
	}
}

type processorStore struct {
	source     []byte
	gets, puts int
}

func (s *processorStore) Get(context.Context, media.ObjectRef) (io.ReadCloser, error) {
	s.gets++
	return io.NopCloser(bytes.NewReader(s.source)), nil
}
func (s *processorStore) Put(_ context.Context, request media.PutRequest) (media.StoredObject, error) {
	s.puts++
	data, err := io.ReadAll(request.Body)
	if err != nil {
		return media.StoredObject{}, err
	}
	return media.StoredObject{ObjectRef: request.ObjectRef, SizeBytes: int64(len(data))}, nil
}
func (*processorStore) List(context.Context, string) ([]media.ListedObject, error) { return nil, nil }
func (*processorStore) Delete(context.Context, media.ObjectRef) error              { return nil }
func (*processorStore) PublicURL(context.Context, media.ObjectRef) (string, error) { return "", nil }
