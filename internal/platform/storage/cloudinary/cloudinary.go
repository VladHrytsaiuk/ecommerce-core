// Package cloudinary реалізує інтерфейс storage.Storage для роботи з Cloudinary.
package cloudinary

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

// Client — обгортка над Cloudinary SDK.
type Client struct {
	cl *cloudinary.Cloudinary
	l  logger.Logger
}

// New створює новий інстанс клієнта Cloudinary.
func New(url string, l logger.Logger) (*Client, error) {
	if url == "" {
		return nil, fmt.Errorf("cloudinary: URL cannot be empty")
	}
	cl, err := cloudinary.NewFromURL(url)
	if err != nil {
		return nil, fmt.Errorf("cloudinary: failed to initialize: %w", err)
	}
	return &Client{
		cl: cl,
		l:  l,
	}, nil
}

// Upload завантажує файл у Cloudinary.
func (c *Client) Upload(ctx context.Context, file interface{}, folder, filename string) (string, error) {
	publicID := filename
	if extIdx := strings.LastIndex(publicID, "."); extIdx != -1 {
		publicID = publicID[:extIdx]
	}

	// Визначаємо тип ресурсу: для SVG примусово ставимо "image"
	isSVG := strings.HasSuffix(strings.ToLower(filename), ".svg")
	resourceType := "auto"
	if isSVG {
		resourceType = "image"
	}

	params := uploader.UploadParams{
		Folder:         folder,
		PublicID:       publicID,
		Overwrite:      getBoolPtr(true),
		UniqueFilename: getBoolPtr(false), // Не додаємо суфікс, щоб зберегти детерміновані посилання (наприклад, для логотипа)
		ResourceType:   resourceType,
	}
	if isSVG {
		params.Format = "svg"
	}

	resp, err := c.cl.Upload.Upload(ctx, file, params)
	if err != nil {
		c.l.Errorw("cloudinary: network/http error during upload", "folder", folder, "filename", filename, "error", err)
		return "", fmt.Errorf("cloudinary: network error: %w", err)
	}

	if resp.Error.Message != "" {
		c.l.Errorw("cloudinary: API error during upload", "folder", folder, "filename", filename, "api_error", resp.Error.Message)
		return "", fmt.Errorf("cloudinary: API error: %s", resp.Error.Message)
	}

	url := resp.SecureURL
	// Якщо це SVG, додаємо прапорець fl_sanitize для безпечного inline відображення у браузері
	// та гарантуємо наявність розширення .svg в URL, щоб браузер відображав його inline
	if isSVG && url != "" {
		if !strings.HasSuffix(strings.ToLower(url), ".svg") {
			url += ".svg"
		}
		url = strings.Replace(url, "/image/upload/", "/image/upload/fl_sanitize/", 1)
	}

	c.l.Infow("cloudinary: upload successful", "folder", folder, "filename", filename, "url", url)
	return url, nil
}

// Delete видаляє файл з Cloudinary.
func (c *Client) Delete(ctx context.Context, publicID string) error {
	_, err := c.cl.Upload.Destroy(ctx, uploader.DestroyParams{
		PublicID: publicID,
	})
	if err != nil {
		c.l.Errorw("cloudinary: delete failed", "public_id", publicID, "error", err)
		return fmt.Errorf("cloudinary: failed to delete: %w", err)
	}

	c.l.Infow("cloudinary: delete successful", "public_id", publicID)
	return nil
}

func getBoolPtr(b bool) *bool {
	return &b
}
