package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
)

const PermissionMediaWrite = "media:write"

type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

type UploadCommand struct {
	ActorID        uuid.UUID
	UploadID       uuid.UUID
	Body           io.Reader
	SizeBytes      int64
	MIMEType       string
	ChecksumSHA256 string
}

type UploadService struct {
	repository       media.AssetRepository
	store            media.ObjectStore
	tx               TransactionManager
	publisher        events.TransactionalEventPublisher
	provider, bucket string
}

func NewUploadService(repository media.AssetRepository, store media.ObjectStore, tx TransactionManager, publisher events.TransactionalEventPublisher, provider, bucket string) (*UploadService, error) {
	if repository == nil || store == nil || tx == nil || publisher == nil || strings.TrimSpace(provider) == "" || strings.TrimSpace(bucket) == "" {
		return nil, fmt.Errorf("media upload dependencies are required")
	}
	return &UploadService{repository: repository, store: store, tx: tx, publisher: publisher, provider: strings.TrimSpace(provider), bucket: strings.TrimSpace(bucket)}, nil
}

func (s *UploadService) Upload(ctx context.Context, command UploadCommand) (media.Asset, error) {
	if command.UploadID == uuid.Nil || command.Body == nil || command.SizeBytes <= 0 || !allowedMIME(command.MIMEType) || len(command.ChecksumSHA256) != 64 {
		return media.Asset{}, fmt.Errorf("invalid media upload")
	}
	if existing, err := s.repository.GetByUploadID(ctx, command.UploadID); err == nil {
		return existing, nil
	} else if !errors.Is(err, media.ErrAssetNotFound) {
		return media.Asset{}, fmt.Errorf("find media upload: %w", err)
	}
	assetID := uuid.NewSHA1(uuid.NameSpaceOID, command.UploadID[:])
	key := "quarantine/" + assetID.String()
	stored, err := s.store.Put(ctx, media.PutRequest{ObjectRef: media.ObjectRef{Provider: s.provider, Bucket: s.bucket, Key: key}, Body: command.Body, ContentType: command.MIMEType, ContentLength: command.SizeBytes, ChecksumSHA256: command.ChecksumSHA256})
	if err != nil {
		return media.Asset{}, fmt.Errorf("store media object: %w", err)
	}
	asset := media.Asset{ID: assetID, UploadID: command.UploadID, Object: stored.ObjectRef, Status: media.AssetQuarantine, ChecksumSHA256: command.ChecksumSHA256, SizeBytes: command.SizeBytes, MIMEType: command.MIMEType, CreatedBy: command.ActorID, CreatedAt: time.Now().UTC()}
	event, err := media.NewAssetUploadedEvent(command.UploadID, assetID, asset.CreatedAt)
	if err != nil {
		_ = s.store.Delete(context.Background(), stored.ObjectRef)
		return media.Asset{}, err
	}
	if err := s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repository.Create(txCtx, asset); err != nil {
			return err
		}
		return s.publisher.Publish(txCtx, event)
	}); err != nil {
		// The blob is quarantined but unreferenced only on a failed local commit;
		// remove it best-effort so retries can use their idempotency key safely.
		_ = s.store.Delete(context.Background(), stored.ObjectRef)
		return media.Asset{}, fmt.Errorf("persist media upload: %w", err)
	}
	return asset, nil
}

func allowedMIME(value string) bool {
	return value == "image/jpeg" || value == "image/png" || value == "image/webp"
}
