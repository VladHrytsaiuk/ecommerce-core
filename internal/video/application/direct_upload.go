// Package application implements video use cases without importing providers.
package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

const PermissionVideoWrite = "video:write"

type DirectUploadService struct {
	repository video.AssetRepository
	provider   video.VideoProvider
	providerID string
	now        func() time.Time
}

func NewDirectUploadService(repository video.AssetRepository, provider video.VideoProvider, providerID string) (*DirectUploadService, error) {
	if repository == nil || provider == nil || strings.TrimSpace(providerID) == "" {
		return nil, fmt.Errorf("video direct upload dependencies are required")
	}
	return &DirectUploadService{repository: repository, provider: provider, providerID: strings.TrimSpace(providerID), now: time.Now}, nil
}

type CreateDirectUploadCommand struct {
	MaxDurationSeconds int
}

func (s *DirectUploadService) CreateDirectUpload(ctx context.Context, command CreateDirectUploadCommand) (video.Asset, video.UploadInstruction, error) {
	if s == nil || command.MaxDurationSeconds < 0 || command.MaxDurationSeconds > 24*60*60 {
		return video.Asset{}, video.UploadInstruction{}, fmt.Errorf("invalid video direct upload request")
	}
	now := s.now().UTC()
	asset := video.Asset{ID: uuid.New(), Provider: s.providerID, Status: video.AssetDraft, CreatedAt: now, UpdatedAt: now}
	// Commit local intent before external I/O. In particular, never keep a
	// PostgreSQL transaction open while Cloudflare receives the request.
	if err := s.repository.CreateDraft(ctx, asset); err != nil {
		return video.Asset{}, video.UploadInstruction{}, fmt.Errorf("create video draft: %w", err)
	}
	instruction, err := s.provider.CreateDirectUpload(ctx, video.DirectUploadMetadata{AssetID: asset.ID, MaxDurationSeconds: command.MaxDurationSeconds})
	if err != nil {
		_ = s.repository.MarkFailed(context.WithoutCancel(ctx), asset.ID)
		return video.Asset{}, video.UploadInstruction{}, fmt.Errorf("create provider direct upload: %w", err)
	}
	if strings.TrimSpace(instruction.ExternalID) == "" || strings.TrimSpace(instruction.UploadURL) == "" {
		_ = s.repository.MarkFailed(context.WithoutCancel(ctx), asset.ID)
		return video.Asset{}, video.UploadInstruction{}, fmt.Errorf("provider returned incomplete direct upload instruction")
	}
	if err := s.repository.MarkUploading(ctx, asset.ID, instruction.ExternalID); err != nil {
		// The database record has no external ID in this failure mode, so a later
		// reconciler cannot identify the provider-side object. Best-effort cleanup
		// is deliberately outside a SQL transaction.
		_ = s.provider.DeleteAsset(context.WithoutCancel(ctx), instruction.ExternalID)
		return video.Asset{}, video.UploadInstruction{}, fmt.Errorf("persist video upload instruction: %w", err)
	}
	asset.ExternalID, asset.Status, asset.UpdatedAt = instruction.ExternalID, video.AssetUploading, s.now().UTC()
	return asset, instruction, nil
}
