package application

import (
	"context"
	"errors"
	"fmt"

	events "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

// WebhookService verifies and normalizes external input before opening its
// short local database transaction. Provider network I/O never runs in it.
type WebhookService struct {
	repository video.AssetRepository
	provider   video.VideoProvider
	tx         TransactionManager
	publisher  events.TransactionalEventPublisher
}

func NewWebhookService(repository video.AssetRepository, provider video.VideoProvider, tx TransactionManager, publisher events.TransactionalEventPublisher) (*WebhookService, error) {
	if repository == nil || provider == nil || tx == nil || publisher == nil {
		return nil, fmt.Errorf("video webhook dependencies are required")
	}
	return &WebhookService{repository: repository, provider: provider, tx: tx, publisher: publisher}, nil
}

func (s *WebhookService) Handle(ctx context.Context, rawPayload []byte, signature string) error {
	if len(rawPayload) == 0 {
		return video.ErrInvalidWebhookPayload
	}
	if err := s.provider.VerifyWebhookSignature(ctx, rawPayload, signature); err != nil {
		return err
	}
	event, err := s.provider.ParseWebhook(ctx, rawPayload)
	if err != nil {
		return err
	}
	return s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		asset, err := s.repository.GetByExternalIDForUpdate(txCtx, event.ExternalID)
		if errors.Is(err, video.ErrAssetNotFound) {
			// A valid account-level webhook may describe a Stream video created
			// outside this application. It is safely irrelevant to this module.
			return nil
		}
		if err != nil {
			return fmt.Errorf("lock video asset: %w", err)
		}
		if asset.Status == video.AssetReady || asset.Status == video.AssetFailed || asset.Status == video.AssetDeleting || asset.Status == video.AssetDeleted {
			return nil
		}
		switch event.Outcome {
		case video.WebhookEncodingSuccess:
			if err := s.repository.MarkReady(txCtx, asset.ID, event.DurationSeconds, event.PosterURL); err != nil {
				return fmt.Errorf("mark video ready: %w", err)
			}
			readyEvent, err := video.NewVideoReadyEvent(asset.ID, asset.ExternalID, event.OccurredAt)
			if err != nil {
				return err
			}
			if err := s.publisher.Publish(txCtx, readyEvent); err != nil {
				return fmt.Errorf("publish video ready: %w", err)
			}
		case video.WebhookEncodingError:
			if err := s.repository.MarkFailed(txCtx, asset.ID); err != nil {
				return fmt.Errorf("mark video failed: %w", err)
			}
		default:
			return video.ErrInvalidWebhookPayload
		}
		return nil
	})
}
