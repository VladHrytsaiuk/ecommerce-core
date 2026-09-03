// Package postgres persists Video-owned data only.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	transaction "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type assetRecord struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	Provider        string
	ExternalID      *string
	Status          string
	DurationSeconds *int
	PosterURL       *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (assetRecord) TableName() string { return "video_assets" }

func (r *Repository) CreateDraft(ctx context.Context, asset video.Asset) error {
	if r == nil || r.db == nil || asset.ID == uuid.Nil || asset.Provider == "" || asset.Status != video.AssetDraft {
		return fmt.Errorf("invalid video asset draft")
	}
	return r.database(ctx).Create(&assetRecord{ID: asset.ID, Provider: asset.Provider, Status: string(video.AssetDraft), CreatedAt: asset.CreatedAt, UpdatedAt: asset.UpdatedAt}).Error
}

func (r *Repository) MarkUploading(ctx context.Context, id uuid.UUID, externalID string) error {
	if r == nil || r.db == nil || id == uuid.Nil || externalID == "" {
		return fmt.Errorf("invalid video upload instruction")
	}
	result := r.database(ctx).Model(&assetRecord{}).Where("id = ? AND status = ?", id, video.AssetDraft).Updates(map[string]any{"external_id": externalID, "status": video.AssetUploading, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return video.ErrAssetNotFound
	}
	return nil
}

func (r *Repository) MarkFailed(ctx context.Context, id uuid.UUID) error {
	if r == nil || r.db == nil || id == uuid.Nil {
		return fmt.Errorf("invalid video asset ID")
	}
	result := r.database(ctx).Model(&assetRecord{}).Where("id = ? AND status IN ?", id, []video.AssetStatus{video.AssetDraft, video.AssetUploading, video.AssetProcessing}).Updates(map[string]any{"status": video.AssetFailed, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return video.ErrAssetNotFound
	}
	return nil
}

func (r *Repository) GetByExternalIDForUpdate(ctx context.Context, externalID string) (video.Asset, error) {
	if r == nil || r.db == nil || externalID == "" {
		return video.Asset{}, fmt.Errorf("invalid video external ID")
	}
	var row assetRecord
	if err := r.database(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("external_id = ?", externalID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return video.Asset{}, video.ErrAssetNotFound
		}
		return video.Asset{}, err
	}
	asset := video.Asset{ID: row.ID, Provider: row.Provider, Status: video.AssetStatus(row.Status), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.ExternalID != nil {
		asset.ExternalID = *row.ExternalID
	}
	if row.DurationSeconds != nil {
		asset.DurationSeconds = *row.DurationSeconds
	}
	if row.PosterURL != nil {
		asset.PosterURL = *row.PosterURL
	}
	return asset, nil
}

func (r *Repository) MarkReady(ctx context.Context, id uuid.UUID, durationSeconds int, posterURL string) error {
	if r == nil || r.db == nil || id == uuid.Nil || durationSeconds < 0 || len(posterURL) > 2048 {
		return fmt.Errorf("invalid ready video asset")
	}
	result := r.database(ctx).Model(&assetRecord{}).Where("id = ? AND status IN ?", id, []video.AssetStatus{video.AssetUploading, video.AssetProcessing}).Updates(map[string]any{"status": video.AssetReady, "duration_seconds": durationSeconds, "poster_url": posterURL, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return video.ErrAssetNotFound
	}
	return nil
}

// ClaimStaleForCleanup gives each candidate a durable lease before any
// provider call. staleBefore applies to ordinary upload states; reclaimBefore
// recovers a lease abandoned by SIGKILL/OOM without waiting another 48 hours.
func (r *Repository) ClaimStaleForCleanup(ctx context.Context, staleBefore, reclaimBefore time.Time, limit int) ([]video.Asset, error) {
	if r == nil || r.db == nil || staleBefore.IsZero() || reclaimBefore.IsZero() || limit <= 0 || limit > 500 {
		return nil, fmt.Errorf("invalid video cleanup claim")
	}
	var rows []assetRecord
	err := transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		query := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("(status IN ? AND updated_at < ?) OR (status = ? AND updated_at < ?)",
				[]video.AssetStatus{video.AssetDraft, video.AssetUploading, video.AssetProcessing}, staleBefore,
				video.AssetDeleting, reclaimBefore).
			Order("updated_at ASC").Limit(limit)
		if err := query.Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uuid.UUID, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		result := tx.WithContext(ctx).Model(&assetRecord{}).Where("id IN ?", ids).Updates(map[string]any{"status": video.AssetDeleting, "updated_at": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return fmt.Errorf("claim video cleanup assets: expected %d rows, got %d", len(ids), result.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	assets := make([]video.Asset, 0, len(rows))
	for _, row := range rows {
		asset := video.Asset{ID: row.ID, Provider: row.Provider, Status: video.AssetStatus(row.Status), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
		if row.ExternalID != nil {
			asset.ExternalID = *row.ExternalID
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

func (r *Repository) MarkDeleted(ctx context.Context, id uuid.UUID) error {
	if r == nil || r.db == nil || id == uuid.Nil {
		return fmt.Errorf("invalid video asset ID")
	}
	result := r.database(ctx).Model(&assetRecord{}).Where("id = ? AND status = ?", id, video.AssetDeleting).Updates(map[string]any{"status": video.AssetDeleted, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return video.ErrAssetNotFound
	}
	return nil
}

func (r *Repository) RestoreCleanup(ctx context.Context, id uuid.UUID, status video.AssetStatus) error {
	if r == nil || r.db == nil || id == uuid.Nil || (status != video.AssetDraft && status != video.AssetUploading && status != video.AssetProcessing) {
		return fmt.Errorf("invalid video cleanup restore")
	}
	result := r.database(ctx).Model(&assetRecord{}).Where("id = ? AND status = ?", id, video.AssetDeleting).Updates(map[string]any{"status": status, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return video.ErrAssetNotFound
	}
	return nil
}

func (r *Repository) ListReadyProductVideos(ctx context.Context, productID uuid.UUID) ([]video.ProductVideo, error) {
	if r == nil || r.db == nil || productID == uuid.Nil {
		return nil, fmt.Errorf("invalid product ID")
	}
	var rows []struct {
		ID, ProductID, AssetID                uuid.UUID
		Role, Provider, ExternalID, PosterURL string
		Position, DurationSeconds             int
		CreatedAt, UpdatedAt                  time.Time
	}
	query := r.database(ctx).Table("product_videos AS product_video").
		Select("product_video.id, product_video.product_id, product_video.video_asset_id AS asset_id, product_video.role, product_video.position, video_asset.provider, video_asset.external_id, video_asset.duration_seconds, video_asset.poster_url, video_asset.created_at, video_asset.updated_at").
		Joins("JOIN video_assets AS video_asset ON video_asset.id = product_video.video_asset_id").
		Where("product_video.product_id = ? AND product_video.is_visible = ? AND video_asset.status = ?", productID, true, video.AssetReady).
		Order("product_video.position ASC, product_video.id ASC")
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]video.ProductVideo, 0, len(rows))
	for _, row := range rows {
		result = append(result, video.ProductVideo{ID: row.ID, ProductID: row.ProductID, AssetID: row.AssetID, Role: row.Role, Position: row.Position, Asset: video.Asset{ID: row.AssetID, Provider: row.Provider, ExternalID: row.ExternalID, Status: video.AssetReady, DurationSeconds: row.DurationSeconds, PosterURL: row.PosterURL, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}})
	}
	return result, nil
}

func (r *Repository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

var _ video.AssetRepository = (*Repository)(nil)
var _ video.StorefrontReader = (*Repository)(nil)
