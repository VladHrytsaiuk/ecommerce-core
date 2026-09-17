package cloudinary

import (
	"context"
	"testing"

	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
)

func TestNewAndPublicURLWithoutNetworkCall(t *testing.T) {
	adapter, err := New(Config{URL: "cloudinary://key:secret@demo"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	url, err := adapter.PublicURL(context.Background(), media.ObjectRef{Provider: provider, Key: "quarantine/asset"})
	if err != nil {
		t.Fatalf("PublicURL() error = %v", err)
	}
	if url == "" {
		t.Fatal("PublicURL() returned an empty URL")
	}
}

func TestNewRejectsEmptyURL(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("New() error = nil, want missing Cloudinary URL")
	}
}
