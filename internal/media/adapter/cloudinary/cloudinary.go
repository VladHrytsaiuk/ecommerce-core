// Package cloudinary adapts Cloudinary to Media's provider-neutral ObjectStore port.
package cloudinary

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	cloudinarySDK "github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/admin"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"

	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
)

const provider = "cloudinary"

type Config struct {
	URL string
}

type Adapter struct {
	client     *cloudinarySDK.Cloudinary
	httpClient *http.Client
}

func New(cfg Config) (*Adapter, error) {
	url := strings.TrimSpace(cfg.URL)
	if url == "" {
		return nil, fmt.Errorf("Cloudinary URL is required")
	}
	client, err := cloudinarySDK.NewFromURL(url)
	if err != nil {
		return nil, fmt.Errorf("initialize Cloudinary client: %w", err)
	}
	return &Adapter{client: client, httpClient: &http.Client{Timeout: 30 * time.Second}}, nil
}

func (a *Adapter) Put(ctx context.Context, request media.PutRequest) (media.StoredObject, error) {
	if a == nil || a.client == nil || request.Body == nil || !validKey(request.Key) || request.ContentLength <= 0 {
		return media.StoredObject{}, fmt.Errorf("invalid media object put request")
	}
	result, err := a.client.Upload.Upload(ctx, request.Body, uploader.UploadParams{
		PublicID:       request.Key,
		UseFilename:    boolPtr(false),
		UniqueFilename: boolPtr(false),
		Overwrite:      boolPtr(true),
		ResourceType:   "image",
		AllowedFormats: api.CldAPIArray{"jpg", "jpeg", "png", "webp"},
	})
	if err != nil {
		return media.StoredObject{}, fmt.Errorf("put Cloudinary media object: %w", err)
	}
	if result.PublicID == "" {
		return media.StoredObject{}, fmt.Errorf("Cloudinary upload returned no public ID")
	}
	return media.StoredObject{
		ObjectRef: media.ObjectRef{Provider: provider, Key: result.PublicID},
		ETag:      result.Etag,
		SizeBytes: int64(result.Bytes),
		Width:     result.Width,
		Height:    result.Height,
	}, nil
}

// Get uses Cloudinary's delivery URL, which is derived exclusively from this
// adapter's configured account and a server-generated public ID. The request
// remains context-aware so worker shutdown and deadlines interrupt downloads.
func (a *Adapter) Get(ctx context.Context, object media.ObjectRef) (io.ReadCloser, error) {
	if a == nil || a.httpClient == nil || !validKey(object.Key) {
		return nil, fmt.Errorf("invalid Cloudinary media object read")
	}
	url, err := a.PublicURL(ctx, object)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build Cloudinary media request: %w", err)
	}
	response, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get Cloudinary media object: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_ = response.Body.Close()
		return nil, fmt.Errorf("get Cloudinary media object: unexpected status %d", response.StatusCode)
	}
	return response.Body, nil
}

func (a *Adapter) List(ctx context.Context, prefix string) ([]media.ListedObject, error) {
	if a == nil || a.client == nil {
		return nil, fmt.Errorf("invalid Cloudinary media object listing")
	}
	prefix = strings.TrimSpace(prefix)
	objects := make([]media.ListedObject, 0)
	var cursor string
	for {
		result, err := a.client.Admin.Assets(ctx, admin.AssetsParams{
			AssetType: api.Image, Prefix: prefix, NextCursor: cursor, MaxResults: 500,
		})
		if err != nil {
			return nil, fmt.Errorf("list Cloudinary media objects: %w", err)
		}
		for _, asset := range result.Assets {
			if !validKey(asset.PublicID) {
				continue
			}
			objects = append(objects, media.ListedObject{ObjectRef: media.ObjectRef{Provider: provider, Key: asset.PublicID}, LastModified: asset.CreatedAt})
		}
		if result.NextCursor == "" {
			return objects, nil
		}
		cursor = result.NextCursor
	}
}

func (a *Adapter) Delete(ctx context.Context, object media.ObjectRef) error {
	if a == nil || a.client == nil || !validKey(object.Key) {
		return fmt.Errorf("invalid Cloudinary media object deletion")
	}
	_, err := a.client.Upload.Destroy(ctx, uploader.DestroyParams{
		PublicID: object.Key, ResourceType: "image", Invalidate: boolPtr(true),
	})
	if err != nil {
		return fmt.Errorf("delete Cloudinary media object: %w", err)
	}
	return nil
}

func (a *Adapter) PublicURL(_ context.Context, object media.ObjectRef) (string, error) {
	if a == nil || a.client == nil || !validKey(object.Key) {
		return "", fmt.Errorf("invalid Cloudinary media object URL")
	}
	asset, err := a.client.Image(object.Key)
	if err != nil {
		return "", fmt.Errorf("create Cloudinary media URL: %w", err)
	}
	url, err := asset.String()
	if err != nil {
		return "", fmt.Errorf("render Cloudinary media URL: %w", err)
	}
	return url, nil
}

func validKey(key string) bool {
	key = strings.TrimSpace(key)
	return key != "" && !strings.Contains(key, "..") && !strings.ContainsAny(key, "\x00\r\n")
}

func boolPtr(value bool) *bool { return &value }

var _ media.ObjectStore = (*Adapter)(nil)
