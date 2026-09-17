package application

import (
	"context"
	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
	"github.com/google/uuid"
)

type CatalogReader struct {
	repo  media.AssetRepository
	store media.ObjectStore
}

func NewCatalogReader(r media.AssetRepository, s media.ObjectStore) *CatalogReader {
	return &CatalogReader{r, s}
}
func (c *CatalogReader) CheckAssetsReady(ctx context.Context, ids []uuid.UUID) error {
	return c.repo.CheckAssetsReady(ctx, ids)
}
func (c *CatalogReader) GetPublicURLs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	vs, err := c.repo.VariantsForAssets(ctx, ids)
	if err != nil {
		return nil, err
	}
	for id, items := range vs {
		for _, v := range items {
			if v.Key == "product" {
				u, e := c.store.PublicURL(ctx, v.Object)
				if e != nil {
					return nil, e
				}
				out[id] = u
				break
			}
		}
	}
	return out, nil
}
