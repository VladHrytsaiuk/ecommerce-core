// Package postgres persists Media-owned state without reaching into Catalog.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
	transaction "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type assetRecord struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	UploadID       uuid.UUID
	Provider       string
	Bucket         string
	ObjectKey      string
	Status         string
	ChecksumSHA256 string
	SizeBytes      int64
	MIMEType       string
	CreatedBy      *uuid.UUID
	CreatedAt      time.Time
}

func (assetRecord) TableName() string { return "media_assets" }

func (r *Repository) Create(ctx context.Context, asset media.Asset) error {
	if r == nil || r.db == nil || asset.ID == uuid.Nil || asset.UploadID == uuid.Nil || asset.Object.Key == "" || asset.SizeBytes <= 0 || asset.ChecksumSHA256 == "" || asset.Status != media.AssetQuarantine {
		return fmt.Errorf("invalid media asset")
	}
	db := r.db.WithContext(ctx)
	if tx, err := transaction.FromContext(ctx); err == nil {
		db = tx.WithContext(ctx)
	}
	var createdBy *uuid.UUID
	if asset.CreatedBy != uuid.Nil {
		createdBy = &asset.CreatedBy
	}
	return db.Create(&assetRecord{ID: asset.ID, UploadID: asset.UploadID, Provider: asset.Object.Provider, Bucket: asset.Object.Bucket, ObjectKey: asset.Object.Key, Status: string(asset.Status), ChecksumSHA256: asset.ChecksumSHA256, SizeBytes: asset.SizeBytes, MIMEType: asset.MIMEType, CreatedBy: createdBy, CreatedAt: asset.CreatedAt}).Error
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (media.Asset, error) {
	if r == nil || r.db == nil || id == uuid.Nil {
		return media.Asset{}, fmt.Errorf("invalid media asset ID")
	}
	var row assetRecord
	db := r.db.WithContext(ctx)
	if tx, err := transaction.FromContext(ctx); err == nil {
		db = tx.WithContext(ctx)
	}
	if err := db.Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return media.Asset{}, media.ErrAssetNotFound
		}
		return media.Asset{}, err
	}
	var createdBy uuid.UUID
	if row.CreatedBy != nil {
		createdBy = *row.CreatedBy
	}
	return media.Asset{ID: row.ID, UploadID: row.UploadID, Object: media.ObjectRef{Provider: row.Provider, Bucket: row.Bucket, Key: row.ObjectKey}, Status: media.AssetStatus(row.Status), ChecksumSHA256: row.ChecksumSHA256, SizeBytes: row.SizeBytes, MIMEType: row.MIMEType, CreatedBy: createdBy, CreatedAt: row.CreatedAt}, nil
}

func (r *Repository) GetByUploadID(ctx context.Context, uploadID uuid.UUID) (media.Asset, error) {
	if r == nil || r.db == nil || uploadID == uuid.Nil {
		return media.Asset{}, fmt.Errorf("invalid media upload ID")
	}
	var row assetRecord
	db := r.db.WithContext(ctx)
	if tx, err := transaction.FromContext(ctx); err == nil {
		db = tx.WithContext(ctx)
	}
	if err := db.Where("upload_id = ?", uploadID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return media.Asset{}, media.ErrAssetNotFound
		}
		return media.Asset{}, err
	}
	var createdBy uuid.UUID
	if row.CreatedBy != nil {
		createdBy = *row.CreatedBy
	}
	return media.Asset{ID: row.ID, UploadID: row.UploadID, Object: media.ObjectRef{Provider: row.Provider, Bucket: row.Bucket, Key: row.ObjectKey}, Status: media.AssetStatus(row.Status), ChecksumSHA256: row.ChecksumSHA256, SizeBytes: row.SizeBytes, MIMEType: row.MIMEType, CreatedBy: createdBy, CreatedAt: row.CreatedAt}, nil
}

func (r *Repository) CompleteProcessing(ctx context.Context, assetID, eventID uuid.UUID, variants []media.Variant) error {
	if assetID == uuid.Nil || eventID == uuid.Nil || len(variants) == 0 {
		return fmt.Errorf("invalid media processing completion")
	}
	return transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_id"}}, DoNothing: true}).Table("media_processing_attempts").Create(map[string]any{"id": uuid.New(), "asset_id": assetID, "event_id": eventID, "attempt": 1, "status": "success", "completed_at": time.Now().UTC()})
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		for _, v := range variants {
			if err := tx.Exec(`INSERT INTO media_variants (asset_id,variant_key,provider,bucket,object_key,width,height,size_bytes,mime_type) VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT (asset_id,variant_key) DO UPDATE SET object_key=EXCLUDED.object_key,size_bytes=EXCLUDED.size_bytes`, assetID, v.Key, v.Object.Provider, v.Object.Bucket, v.Object.Key, v.Width, v.Height, v.SizeBytes, v.MIMEType).Error; err != nil {
				return err
			}
		}
		result = tx.Exec(`UPDATE media_assets SET status='ready',updated_at=CURRENT_TIMESTAMP WHERE id=? AND status IN ('quarantine','processing')`, assetID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("media asset %s is not processable", assetID)
		}
		return nil
	})
}
func (r *Repository) CheckAssetsReady(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	var count int64
	if err := r.database(ctx).Table("media_assets").Where("id IN ? AND status='ready'", ids).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return fmt.Errorf("one or more media assets are not ready")
	}
	return nil
}
func (r *Repository) VariantsForAssets(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]media.Variant, error) {
	out := map[uuid.UUID][]media.Variant{}
	var rows []struct {
		AssetID                                           uuid.UUID
		VariantKey, Provider, Bucket, ObjectKey, MIMEType string
		Width, Height                                     int
		SizeBytes                                         int64
	}
	if len(ids) == 0 {
		return out, nil
	}
	if err := r.database(ctx).Table("media_variants").Where("asset_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, x := range rows {
		out[x.AssetID] = append(out[x.AssetID], media.Variant{AssetID: x.AssetID, Key: x.VariantKey, Object: media.ObjectRef{Provider: x.Provider, Bucket: x.Bucket, Key: x.ObjectKey}, Width: x.Width, Height: x.Height, SizeBytes: x.SizeBytes, MIMEType: x.MIMEType})
	}
	return out, nil
}

// ReferencedObjectKeys checks a complete batch in one query. Failed assets
// deliberately do not retain quarantine objects: they are eligible for the
// bounded orphan-cleanup lifecycle after its grace period.
func (r *Repository) ReferencedObjectKeys(ctx context.Context, keys []string) (map[string]bool, error) {
	referenced := make(map[string]bool, len(keys))
	if len(keys) == 0 {
		return referenced, nil
	}
	var rows []struct{ ObjectKey string }
	if err := r.database(ctx).Table("media_assets").Select("object_key").Where("object_key IN ? AND status <> ?", keys, media.AssetFailed).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		referenced[row.ObjectKey] = true
	}
	return referenced, nil
}
func (r *Repository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

// MarkReady is idempotent by event ID. A duplicate Outbox delivery cannot
// create a second processing attempt or regress a ready asset.
func (r *Repository) MarkReady(ctx context.Context, assetID, eventID uuid.UUID) error {
	if r == nil || r.db == nil || assetID == uuid.Nil || eventID == uuid.Nil {
		return fmt.Errorf("invalid media processing completion")
	}
	return transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		attempt := struct {
			ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
			AssetID     uuid.UUID
			EventID     uuid.UUID
			Attempt     int
			Status      string
			CompletedAt time.Time
		}{ID: uuid.New(), AssetID: assetID, EventID: eventID, Attempt: 1, Status: "success", CompletedAt: time.Now().UTC()}
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_id"}}, DoNothing: true}).Table("media_processing_attempts").Create(&attempt)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		result = tx.Exec(`UPDATE media_assets SET status = 'ready', failure_code = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status IN ('quarantine', 'processing')`, assetID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("media asset %s is not processable", assetID)
		}
		return nil
	})
}

var _ media.AssetRepository = (*Repository)(nil)
