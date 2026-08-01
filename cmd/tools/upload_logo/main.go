// Пакет main реалізує утиліту для одноразового завантаження логотипу магазину в Cloudinary.
package main

import (
	"context"
	"fmt"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/storage/cloudinary"
)

func main() {
	// 1. Завантаження конфігурації
	cfg := config.Load()

	// 2. Ініціалізація логера
	logger.Init()
	l := logger.Log

	l.Info("Starting logo upload to Cloudinary...")

	// 3. Ініціалізація клієнта Cloudinary
	storageClient, err := cloudinary.New(cfg.CloudinaryURL, l)
	if err != nil {
		l.Fatalw("Failed to init cloudinary client", "error", err)
	}

	// 4. Завантаження
	filePath := "docs/logo.png"
	ctx := context.Background()
	folder := "aquawheel/system"
	filename := "logo"

	l.Infow("Uploading file", "path", filePath, "folder", folder, "filename", filename)

	url, err := storageClient.Upload(ctx, filePath, folder, filename)
	if err != nil {
		l.Fatalw("Upload execution failed", "error", err)
	}

	l.Infow("Logo upload completed successfully", "url", url)
	fmt.Printf("\nSUCCESS! Logo URL: %s\n", url)
}
