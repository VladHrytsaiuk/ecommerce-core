package app

import (
	"context"
	"fmt"

	mediaCloudinary "github.com/VladHrytsaiuk/ecommerce-core/internal/media/adapter/cloudinary"
	mediaS3 "github.com/VladHrytsaiuk/ecommerce-core/internal/media/adapter/s3"
	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
)

// newMediaObjectStore is kept in the composition root: application services
// depend only on media.ObjectStore and never on a provider SDK.
func newMediaObjectStore(cfg *config.Config) (media.ObjectStore, string, error) {
	switch cfg.MediaProvider {
	case "cloudinary":
		store, err := mediaCloudinary.New(mediaCloudinary.Config{URL: cfg.MediaCloudinaryURL})
		if err != nil {
			return nil, "", err
		}
		return store, "", nil
	case "s3", "r2", "minio":
		// Supported below by the S3-compatible adapter.
	default:
		return nil, "", fmt.Errorf("unsupported media provider %q", cfg.MediaProvider)
	}
	publicBaseURL := cfg.MediaS3PublicBaseURL
	if cfg.MediaProvider == "r2" {
		publicBaseURL = cfg.MediaR2PublicBaseURL
	}
	store, err := mediaS3.New(context.Background(), mediaS3.Config{
		Provider: cfg.MediaProvider, Bucket: cfg.MediaS3Bucket, Region: cfg.MediaS3Region,
		Endpoint: cfg.MediaS3Endpoint, AccessKeyID: cfg.MediaS3AccessKeyID,
		SecretAccessKey: cfg.MediaS3SecretAccessKey, PublicBaseURL: publicBaseURL,
		UsePathStyle: cfg.MediaS3UsePathStyle,
	})
	if err != nil {
		return nil, "", fmt.Errorf("configure S3-compatible media object store: %w", err)
	}
	return store, cfg.MediaS3Bucket, nil
}
