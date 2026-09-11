package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

// StorefrontService turns stored placements into playable ones. Signing lives
// here rather than in the HTTP adapter so the token's lifetime is a policy of
// the module, not of whichever transport happens to ask.
type StorefrontService struct {
	reader video.StorefrontReader
	signer video.PlaybackSigner
	ttl    time.Duration
	now    func() time.Time
}

func NewStorefrontService(reader video.StorefrontReader, signer video.PlaybackSigner, ttl time.Duration) (*StorefrontService, error) {
	if reader == nil || signer == nil {
		return nil, fmt.Errorf("video storefront dependencies are required")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("video playback TTL must be positive")
	}
	return &StorefrontService{reader: reader, signer: signer, ttl: ttl, now: func() time.Time { return time.Now().UTC() }}, nil
}

// ListPlayableProductVideos returns only what a visitor may actually stream.
//
// A placement whose token cannot be minted is omitted rather than failing the
// whole response: one unplayable video should degrade that video, not hide the
// rest of the product's media behind an error.
func (s *StorefrontService) ListPlayableProductVideos(ctx context.Context, productID uuid.UUID) ([]video.PlayableVideo, error) {
	if s == nil {
		return nil, fmt.Errorf("video storefront service is not configured")
	}
	if productID == uuid.Nil {
		return nil, video.ErrInvalidPlacement
	}
	placements, err := s.reader.ListReadyProductVideos(ctx, productID)
	if err != nil {
		return nil, err
	}
	expiresAt := s.now().Add(s.ttl)
	playable := make([]video.PlayableVideo, 0, len(placements))
	for _, placement := range placements {
		grant, signErr := s.signer.SignPlayback(ctx, placement.Asset.ExternalID, expiresAt)
		if signErr != nil {
			continue
		}
		playable = append(playable, video.PlayableVideo{ProductVideo: placement, Playback: grant})
	}
	return playable, nil
}
