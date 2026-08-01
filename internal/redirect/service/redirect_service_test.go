package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/domain"
	"go.uber.org/zap"
)

type mockLogger struct{}

func (m *mockLogger) Debug(msg string, fields ...zap.Field)       {}
func (m *mockLogger) Info(msg string, fields ...zap.Field)        {}
func (m *mockLogger) Warn(msg string, fields ...zap.Field)        {}
func (m *mockLogger) Error(msg string, fields ...zap.Field)       {}
func (m *mockLogger) Fatal(msg string, fields ...zap.Field)       {}
func (m *mockLogger) Debugf(template string, args ...interface{}) {}
func (m *mockLogger) Infof(template string, args ...interface{})  {}
func (m *mockLogger) Warnf(template string, args ...interface{})  {}
func (m *mockLogger) Errorf(template string, args ...interface{}) {}
func (m *mockLogger) Fatalf(template string, args ...interface{}) {}
func (m *mockLogger) Debugw(msg string, kvs ...interface{})       {}
func (m *mockLogger) Infow(msg string, kvs ...interface{})        {}
func (m *mockLogger) Warnw(msg string, kvs ...interface{})        {}
func (m *mockLogger) Errorw(msg string, kvs ...interface{})       {}
func (m *mockLogger) Fatalw(msg string, kvs ...interface{})       {}
func (m *mockLogger) With(fields ...zap.Field) logger.Logger      { return m }
func (m *mockLogger) Sync() error                                 { return nil }

type mockRedirectRepo struct {
	history     *domain.UrlHistory
	currentSlug string
	saveErr     error
	findErr     error
	currentErr  error
}

func (m *mockRedirectRepo) SaveHistory(ctx context.Context, history *domain.UrlHistory) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.history = history
	return nil
}

func (m *mockRedirectRepo) FindByOldSlug(ctx context.Context, oldSlug string) (*domain.UrlHistory, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	return m.history, nil
}

func (m *mockRedirectRepo) FindCurrentSlug(ctx context.Context, entityType string, entityID uuid.UUID, lang string) (string, error) {
	if m.currentErr != nil {
		return "", m.currentErr
	}
	return m.currentSlug, nil
}

func TestRedirectService_RecordSlugChange(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		repo := &mockRedirectRepo{}
		service := NewRedirectService(repo, &mockLogger{})

		entityID := uuid.New()
		err := service.RecordSlugChange(ctx, "product", entityID, "old-slug")
		assert.NoError(t, err)
		assert.NotNil(t, repo.history)
		assert.Equal(t, "product", repo.history.EntityType)
		assert.Equal(t, entityID, repo.history.EntityID)
		assert.Equal(t, "old-slug", repo.history.OldSlug)
	})

	t.Run("EmptyOldSlug", func(t *testing.T) {
		repo := &mockRedirectRepo{}
		service := NewRedirectService(repo, &mockLogger{})

		err := service.RecordSlugChange(ctx, "product", uuid.New(), "")
		assert.NoError(t, err)
		assert.Nil(t, repo.history)
	})

	t.Run("SaveError", func(t *testing.T) {
		repo := &mockRedirectRepo{saveErr: errors.New("db error")}
		service := NewRedirectService(repo, &mockLogger{})

		err := service.RecordSlugChange(ctx, "product", uuid.New(), "old-slug")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
	})
}

func TestRedirectService_ResolveRedirect(t *testing.T) {
	ctx := context.Background()
	entityID := uuid.New()

	t.Run("Found", func(t *testing.T) {
		repo := &mockRedirectRepo{
			history: &domain.UrlHistory{
				EntityType: "product",
				EntityID:   entityID,
				OldSlug:    "old-slug",
			},
			currentSlug: "new-slug",
		}
		service := NewRedirectService(repo, &mockLogger{})

		entityType, newSlug, err := service.ResolveRedirect(ctx, "old-slug", "uk")
		assert.NoError(t, err)
		assert.Equal(t, "product", entityType)
		assert.Equal(t, "new-slug", newSlug)
	})

	t.Run("NotFound", func(t *testing.T) {
		repo := &mockRedirectRepo{
			history: nil,
		}
		service := NewRedirectService(repo, &mockLogger{})

		entityType, newSlug, err := service.ResolveRedirect(ctx, "non-existent", "uk")
		assert.NoError(t, err)
		assert.Empty(t, entityType)
		assert.Empty(t, newSlug)
	})

	t.Run("FindError", func(t *testing.T) {
		repo := &mockRedirectRepo{
			findErr: errors.New("db error"),
		}
		service := NewRedirectService(repo, &mockLogger{})

		_, _, err := service.ResolveRedirect(ctx, "old-slug", "uk")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to search old slug")
	})

	t.Run("CurrentSlugError", func(t *testing.T) {
		repo := &mockRedirectRepo{
			history: &domain.UrlHistory{
				EntityType: "product",
				EntityID:   entityID,
				OldSlug:    "old-slug",
			},
			currentErr: errors.New("db error"),
		}
		service := NewRedirectService(repo, &mockLogger{})

		_, _, err := service.ResolveRedirect(ctx, "old-slug", "uk")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get current slug")
	})
}
