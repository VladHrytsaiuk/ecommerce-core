package s3

import (
	"context"
	"testing"
)

func TestNewRejectsNonHTTPPublicURL(t *testing.T) {
	_, err := New(context.Background(), Config{Provider: "r2", Bucket: "media", Region: "auto", PublicBaseURL: "javascript:alert(1)"})
	if err == nil {
		t.Fatal("New() error = nil, want invalid public URL")
	}
}

func TestNewBuildsS3CompatibleClientWithoutNetworkCall(t *testing.T) {
	adapter, err := New(context.Background(), Config{Provider: "minio", Bucket: "media", Region: "us-east-1", Endpoint: "http://minio:9000", PublicBaseURL: "https://cdn.example.test/media", UsePathStyle: true})
	if err != nil || adapter == nil {
		t.Fatalf("New() = (%v, %v)", adapter, err)
	}
}
