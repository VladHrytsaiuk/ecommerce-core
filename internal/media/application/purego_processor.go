package application

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"

	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
	"github.com/deepteams/webp"
	"github.com/disintegration/imaging"
)

const (
	MaxImageDimension = 8192
	MaxImagePixels    = int64(16_000_000)
)

type PureGoProcessor struct{ store media.ObjectStore }

func NewPureGoProcessor(store media.ObjectStore) *PureGoProcessor {
	return &PureGoProcessor{store: store}
}
func (p *PureGoProcessor) Process(ctx context.Context, source media.SourceObject) ([]media.StoredObject, error) {
	configReader, err := p.store.Get(ctx, source.Object)
	if err != nil {
		return nil, err
	}
	config, _, configErr := image.DecodeConfig(configReader)
	closeErr := configReader.Close()
	if configErr != nil {
		return nil, fmt.Errorf("decode image config: %w", configErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close image config reader: %w", closeErr)
	}
	if err := validateImageConfig(config); err != nil {
		return nil, err
	}

	// DecodeConfig reads only format metadata. Re-open the S3 object only after
	// the dimension/pixel guard accepts it, preventing decompression bombs from
	// allocating their full pixel buffer in the worker.
	r, err := p.store.Get(ctx, source.Object)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	specs := []struct {
		k   string
		max int
	}{{"thumbnail", 400}, {"product", 1000}}
	out := make([]media.StoredObject, 0, 2)
	for _, s := range specs {
		resized := imaging.Fit(img, s.max, s.max, imaging.Lanczos)
		var b bytes.Buffer
		if err := webp.Encode(&b, resized, &webp.EncoderOptions{Quality: 82, Method: 4}); err != nil {
			return nil, err
		}
		key := fmt.Sprintf("variants/%s/%s.webp", source.AssetID, s.k)
		stored, err := p.store.Put(ctx, media.PutRequest{ObjectRef: media.ObjectRef{Provider: source.Object.Provider, Bucket: source.Object.Bucket, Key: key}, Body: bytes.NewReader(b.Bytes()), ContentType: "image/webp", ContentLength: int64(b.Len())})
		if err != nil {
			return nil, err
		}
		stored.SizeBytes = int64(b.Len())
		stored.Width = resized.Bounds().Dx()
		stored.Height = resized.Bounds().Dy()
		out = append(out, stored)
	}
	return out, nil
}

func validateImageConfig(config image.Config) error {
	if config.Width <= 0 || config.Height <= 0 || config.Width > MaxImageDimension || config.Height > MaxImageDimension {
		return fmt.Errorf("image dimensions exceed %dx%d", MaxImageDimension, MaxImageDimension)
	}
	if int64(config.Width)*int64(config.Height) > MaxImagePixels {
		return fmt.Errorf("image pixel count exceeds %d", MaxImagePixels)
	}
	return nil
}

var _ media.ImageProcessor = (*PureGoProcessor)(nil)
