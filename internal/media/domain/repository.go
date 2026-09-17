package domain

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrAssetNotFound = errors.New("media asset not found")

type AssetRepository interface {
	Create(context.Context, Asset) error
	Get(context.Context, uuid.UUID) (Asset, error)
	GetByUploadID(context.Context, uuid.UUID) (Asset, error)
	CompleteProcessing(context.Context, uuid.UUID, uuid.UUID, []Variant) error
	CheckAssetsReady(context.Context, []uuid.UUID) error
	VariantsForAssets(context.Context, []uuid.UUID) (map[uuid.UUID][]Variant, error)
	ReferencedObjectKeys(context.Context, []string) (map[string]bool, error)
}
