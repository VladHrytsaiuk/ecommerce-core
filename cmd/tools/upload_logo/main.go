// Пакет main реалізує утиліту для одноразового завантаження логотипу магазину в Cloudinary.
package main

import (
	"context"
	"fmt"
	"os"

	mediaCloudinary "github.com/VladHrytsaiuk/ecommerce-core/internal/media/adapter/cloudinary"
	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

func main() {
	// 1. Завантаження конфігурації
	cfg := config.Load()

	// 2. Ініціалізація логера
	logger.Init()
	l := logger.Log

	l.Info("Starting logo upload to Cloudinary...")

	// 3. Use the same ObjectStore implementation as the Media module instead
	// of retaining a parallel legacy Cloudinary client.
	storageClient, err := mediaCloudinary.New(mediaCloudinary.Config{URL: cfg.MediaCloudinaryURL})
	if err != nil {
		l.Fatalw("Failed to init cloudinary client", "error", err)
	}

	// 4. Завантаження
	filePath := "docs/logo.png"
	ctx := context.Background()
	key := "system/logo"

	l.Infow("Uploading file", "path", filePath, "key", key)
	file, err := os.Open(filePath)
	if err != nil {
		l.Fatalw("Failed to open logo", "error", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		l.Fatalw("Failed to stat logo", "error", err)
	}

	stored, err := storageClient.Put(ctx, media.PutRequest{ObjectRef: media.ObjectRef{Provider: "cloudinary", Key: key}, Body: file, ContentType: "image/png", ContentLength: info.Size()})
	if err != nil {
		l.Fatalw("Upload execution failed", "error", err)
	}
	url, err := storageClient.PublicURL(ctx, stored.ObjectRef)
	if err != nil {
		l.Fatalw("Failed to build logo URL", "error", err)
	}

	l.Infow("Logo upload completed successfully", "url", url)
	fmt.Printf("\nSUCCESS! Logo URL: %s\n", url)
}
